package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
)

type UsersRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewUsers(db *sqlx.DB, c *trmsqlx.CtxGetter) *UsersRepository {
	return &UsersRepository{
		db:     db,
		getter: c,
	}
}

func (r *UsersRepository) GetUserById(ctx context.Context, id int) (models.User, error) {
	const op = "storage.postgres.GetUserById"

	var user models.User

	query := `
			SELECT 
				user_id, name, login, password_hash, ST_AsEWKB(home_point) as home_point, rating, role 
			FROM 
				users 
			WHERE 
				user_id = $1
			`
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &user, query, id); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return user, repository.ErrNotFound
		default:
			return user, fmt.Errorf("%s: %w", op, err)
		}
	}

	return user, nil
}

func (r *UsersRepository) GetUserByLogin(ctx context.Context, username string) (models.User, error) {
	const op = "storage.postgres.GetUserByUsername"

	var user models.User

	query := `
			SELECT
				user_id, name, login, password_hash, ST_AsEWKB(home_point) as home_point, rating, role 
			FROM 
				users 
			WHERE 
				login = $1
			`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &user, query, username); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return user, repository.ErrNotFound
		default:
			return user, fmt.Errorf("%s: %w", op, err)
		}
	}
	return user, nil

}

func (r *UsersRepository) GetUsers(ctx context.Context, p models.Pagination) (models.Page[models.User], error) {
	const op = "storage.postgres.GetUsers"

	q := newListQuery(
		"user_id, name, login, ST_AsEWKB(home_point) as home_point, rating, role",
		"users",
	).
		OrderBy("user_id ASC").
		Paginate(p)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.User](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

func (r *UsersRepository) AddUser(ctx context.Context, user models.User) (int64, error) {
	const op = "storage.postgres.AddUser"

	var id int64

	query := `
			INSERT INTO 
				users (name, login, password_hash, home_point, role) 
			VALUES 
				($1, $2, $3, ST_GeomFromEWKB($4), $5) 
			RETURNING user_id
			`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, user.Name, user.Login, user.PasswordHash, user.HomePoint, user.Role); err != nil {
		return 0, fmt.Errorf("%s: %w", op, wrapUniqueViolation(err))
	}

	return id, nil
}

// AddRatingEvent records a rating change and applies it to users.rating in
// the same statement, so the aggregate never drifts from the event log.
func (r *UsersRepository) AddRatingEvent(ctx context.Context, event models.RatingEvent) (int64, error) {
	const op = "storage.postgres.AddRatingEvent"

	var id int64

	query := `
			WITH updated AS (
				UPDATE users SET rating = COALESCE(rating, 0) + $2 WHERE user_id = $1
				RETURNING user_id
			)
			INSERT INTO
				rating_events (user_id, delta, reason, mark_id, check_id)
			SELECT
				user_id, $2, $3, $4, $5
			FROM
				updated
			RETURNING id
			`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, event.UserID, event.Delta, event.Reason, event.MarkID, event.CheckID); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return 0, repository.ErrNotFound
		default:
			return 0, fmt.Errorf("%s: %w", op, err)
		}
	}

	return id, nil
}

// GetRatingEvents returns the user's rating history, newest first.
func (r *UsersRepository) GetRatingEvents(ctx context.Context, userId int, p models.Pagination) (models.Page[models.RatingEvent], error) {
	const op = "storage.postgres.GetRatingEvents"

	q := newListQuery("id, user_id, delta, reason, mark_id, check_id, created_at", "rating_events").
		Where("user_id = ?", userId).
		OrderBy("created_at DESC, id DESC").
		Paginate(p)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.RatingEvent](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// GetLeaderboard returns users ordered by rating (highest first). Only the
// public identity is selected: the leaderboard never needs login, home
// point or role, so they cannot leak from here.
func (r *UsersRepository) GetLeaderboard(ctx context.Context, p models.Pagination) (models.Page[models.User], error) {
	const op = "storage.postgres.GetLeaderboard"

	q := newListQuery("user_id, name, COALESCE(rating, 0) AS rating", "users").
		OrderBy("rating DESC, user_id ASC").
		Paginate(p)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.User](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// GetUserStats aggregates the user's marks, checks and tasks. A mark counts
// as confirmed once it has passed the first vote (any status after
// «Неподтверждённая» except «Опровергнута»); correct checks come from the
// rating event log because correctness is only known when a stage resolves.
func (r *UsersRepository) GetUserStats(ctx context.Context, userId int) (models.UserStats, error) {
	const op = "storage.postgres.GetUserStats"

	var stats models.UserStats

	query := `
			SELECT
				COALESCE(u.rating, 0) AS rating,
				(SELECT COUNT(*) FROM marks m WHERE m.user_id = u.user_id) AS marks_total,
				(SELECT COUNT(*) FROM marks m WHERE m.user_id = u.user_id AND m.mark_status_id IN ($2, $3, $4, $5)) AS marks_confirmed,
				(SELECT COUNT(*) FROM marks m WHERE m.user_id = u.user_id AND m.mark_status_id = $6) AS marks_refuted,
				(SELECT COUNT(*) FROM checks c WHERE c.user_id = u.user_id) AS checks_total,
				(SELECT COUNT(*) FROM rating_events e WHERE e.user_id = u.user_id AND e.reason = $7) AS checks_correct,
				(SELECT COUNT(*) FROM tasks t WHERE t.user_id = u.user_id AND t.status_id = $8) AS tasks_completed
			FROM
				users u
			WHERE
				u.user_id = $1
			`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &stats, query, userId,
		models.ConfirmedStatus, models.UnderReviewStatus, models.RediscoveredStatus, models.ClosedStatus,
		models.RefutedStatus, models.RatingReasonCheckCorrect, models.CompletedStatus,
	); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return stats, repository.ErrNotFound
		default:
			return stats, fmt.Errorf("%s: %w", op, err)
		}
	}

	return stats, nil
}
