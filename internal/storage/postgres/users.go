package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/jmoiron/sqlx"
)

type UsersRepository interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUserByUsername(ctx context.Context, username string) (models.User, error)
	GetUsers(ctx context.Context) ([]models.User, error)
	AddUser(ctx context.Context, user models.User) (int64, error)
}

type UsersRepo struct {
	Conn *sqlx.DB
}

func NewUsers(conn *sqlx.DB) *UsersRepo {
	return &UsersRepo{Conn: conn}
}

func (r *UsersRepo) GetUserById(ctx context.Context, id int) (models.User, error) {
	const op = "storage.postgres.GetUserById"

	var user models.User

	query := "SELECT * FROM users WHERE user_id = $1"
	if err := r.Conn.GetContext(ctx, &user, query, id); err != nil {
		switch err {
		case sql.ErrNoRows:
			return user, storage.ErrNotFound
		default:
			return user, fmt.Errorf("%s: %w", op, err)
		}
	}

	return user, nil
}

func (r *UsersRepo) GetUserByUsername(ctx context.Context, username string) (models.User, error) {
	const op = "storage.postgres.GetUserByUsername"

	var user models.User
	if err := r.Conn.GetContext(ctx, &user, "SELECT * FROM users WHERE username = ?", username); err != nil {
		return user, fmt.Errorf("%s: %w", op, storage.ErrNotFound)
	}
	return user, nil

}

func (r *UsersRepo) GetUsers(ctx context.Context) ([]models.User, error) {
	const op = "storage.postgres.GetUsers"

	users := make([]models.User, 0)

	query := "SELECT * FROM users"
	if err := r.Conn.SelectContext(ctx, &users, query); err != nil {
		return users, fmt.Errorf("%s: %w", op, err)
	}

	return users, nil
}

func (r *UsersRepo) AddUser(ctx context.Context, user models.User) (int64, error) {
	const op = "storage.postgres.AddUser"

	result, err := r.Conn.NamedExecContext(ctx, "INSERT INTO users (name, login, password_hash) VALUES (:name, :login, :password_hash)", user)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	return id, nil
}
