package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
)

type ChecksRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewChecks(db *sqlx.DB, c *trmsqlx.CtxGetter) *ChecksRepository {
	return &ChecksRepository{
		db:     db,
		getter: c,
	}
}

func (r *ChecksRepository) AddCheck(ctx context.Context, check models.Check) (int64, error) {
	const op = "storage.postgres.AddCheck"

	var id int64

	query := `
			INSERT INTO 
				checks (user_id, mark_id, mark_status_id, mark_status_history_id, comment, result) 
			VALUES 
				($1, $2, $3, $4, $5, $6)
			RETURNING check_id
			`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query,
		check.UserID, check.MarkID, check.MarkStatusId, check.MarkStatusHistoryItemId, check.Comment, check.Result,
	); err != nil {
		return 0, wrapPgError(op, err)
	}

	return id, nil
}

func (r *ChecksRepository) GetCheckById(ctx context.Context, id int) (models.Check, error) {
	const op = "storage.postgres.GetCheckById"

	var check models.Check

	query := `
		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			check_id = $1`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &check, query, id); err != nil {
		return check, wrapPgError(op, err)
	}

	return check, nil
}

const (
	checkColumns = "c.*, u.name AS username"
	checksFrom   = "checks AS c JOIN users AS u ON c.user_id = u.user_id"
)

func (r *ChecksRepository) GetChecksByMarkId(ctx context.Context, markId int, p models.Pagination) (models.Page[models.Check], error) {
	const op = "storage.postgres.GetChecksByMarkId"

	q := newListQuery(checkColumns, checksFrom).
		Where("c.mark_id = ?", markId).
		OrderBy("c.created_at ASC, c.check_id ASC").
		Paginate(p)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Check](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

func (r *ChecksRepository) GetChecksByUserId(ctx context.Context, userId int, p models.Pagination) (models.Page[models.Check], error) {
	const op = "storage.postgres.GetChecksByUserId"

	q := newListQuery(checkColumns, checksFrom).
		Where("c.user_id = ?", userId).
		OrderBy("c.created_at ASC, c.check_id ASC").
		Paginate(p)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Check](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

func (r *ChecksRepository) GetChecksByMarkHistoryId(ctx context.Context, markHistoryId int) ([]models.Check, error) {
	const op = "storage.postgres.GetChecksByMarkHistoryId"

	checks := []models.Check{}

	query := `
		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			c.mark_status_history_id = $1`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &checks, query, markHistoryId); err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	return checks, nil
}

func (r *ChecksRepository) GetChecksByUserIdAndMarkId(ctx context.Context, userId int, markId int) ([]models.Check, error) {
	const op = "storage.postgres.GetChecksByUserIdAndMarkId"

	checks := []models.Check{}

	query := `
		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			c.user_id = $1 AND c.mark_id = $2`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &checks, query, userId, markId); err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	return checks, nil
}

func (r *ChecksRepository) GetChecksByUserIdAndMarkIdSince(ctx context.Context, userId int, markId int, dateTime time.Time) ([]models.Check, error) {
	const op = "storage.postgres.GetChecksSince"

	checks := []models.Check{}

	query := `
		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			c.user_id = $1 AND c.mark_id = $2 AND c.created_at > $3`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &checks, query, userId, markId, dateTime); err != nil {
		return checks, fmt.Errorf("%s: %w", op, err)
	}

	return checks, nil
}

// CountChecksByUserIdSince counts the user's checks created after since
// (used by the daily anti-fraud limit).
func (r *ChecksRepository) CountChecksByUserIdSince(ctx context.Context, userId int, since time.Time) (int, error) {
	const op = "storage.postgres.CountChecksByUserIdSince"

	var n int

	query := `SELECT COUNT(*) FROM checks WHERE user_id = $1 AND created_at > $2`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &n, query, userId, since); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return n, nil
}

func (r *ChecksRepository) GetUserMarkCheck(ctx context.Context, userId int, markStatusHistoryId int) (models.Check, error) {
	const op = "storage.postgres.GetUserMarkCheck"

	check := models.Check{}

	query := `
		WITH RECURSIVE r AS (
			SELECT h.id, h.prev_id, ms.parent_id, ms.name 
			FROM mark_status_history AS h
			JOIN mark_statuses AS ms
			ON ms.mark_status_id = h.new_mark_status_id
			WHERE h.id = $1
		UNION 
			SELECT h2.id, h2.prev_id, ms2.parent_id, ms2.name  
			FROM mark_status_history AS h2
			JOIN mark_statuses AS ms2
			ON ms2.mark_status_id = h2.new_mark_status_id
			JOIN r 
			ON r.prev_id = h2.id
			WHERE r.parent_id = h2.new_mark_status_id
		)

		SELECT 
			c.*, u.name as username 
		FROM 
			checks as c 
		JOIN 
			users AS u ON c.user_id = u.user_id 
		WHERE 
			c.user_id = $2 AND mark_status_history_id IN (SELECT id FROM r)
		ORDER BY
			c.created_at DESC, c.check_id DESC
		LIMIT 1`

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &check, query, markStatusHistoryId, userId); err != nil {
		return check, wrapPgError(op, err)
	}

	return check, nil
}
