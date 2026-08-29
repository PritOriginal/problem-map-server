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
