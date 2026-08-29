package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type UsersRepository interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUserByLogin(ctx context.Context, username string) (models.User, error)
	GetUsers(ctx context.Context, p models.Pagination) (models.Page[models.User], error)
	AddUser(ctx context.Context, user models.User) (int64, error)
}

type Users struct {
	log   *slog.Logger
	repos UsersRepositories
}

type UsersRepositories struct {
	Users UsersRepository
}

func NewUsers(log *slog.Logger, repos UsersRepositories) *Users {
	return &Users{log: log, repos: repos}
}

func (uc *Users) GetUserById(ctx context.Context, id int) (models.User, error) {
	const op = "usecase.Users.GetUserById"

	user, err := uc.repos.Users.GetUserById(ctx, id)
	if err != nil {
		return user, fmt.Errorf("%s: %w", op, err)
	}

	return user, nil
}

// ListUsers returns a page of users with the total count.
func (uc *Users) ListUsers(ctx context.Context, p models.Pagination) (models.Page[models.User], error) {
	const op = "usecase.Users.ListUsers"

	if err := p.Validate(); err != nil {
		return models.Page[models.User]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Users.GetUsers(ctx, p)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// GetUsers returns all users without pagination (gRPC, tasker).
func (uc *Users) GetUsers(ctx context.Context) ([]models.User, error) {
	const op = "usecase.Users.GetUsers"

	page, err := uc.repos.Users.GetUsers(ctx, models.Pagination{})
	if err != nil {
		return page.Items, fmt.Errorf("%s: %w", op, err)
	}

	return page.Items, nil
}
