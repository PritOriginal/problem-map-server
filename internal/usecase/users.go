package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

type UsersRepository interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUserByUsername(ctx context.Context, username string) (models.User, error)
	GetUsers(ctx context.Context) ([]models.User, error)
	AddUser(ctx context.Context, user models.User) (int64, error)
}

type Users struct {
	log       *slog.Logger
	usersRepo UsersRepository
}

func NewUsers(log *slog.Logger, usersRepo UsersRepository) *Users {
	return &Users{log: log, usersRepo: usersRepo}
}

func (uc *Users) GetUserById(ctx context.Context, id int) (models.User, error) {
	const op = "usecase.Users.GetUserById"

	user, err := uc.usersRepo.GetUserById(ctx, id)
	if err != nil {
		return user, fmt.Errorf("%s: %w", op, err)
	}

	return user, nil
}

func (uc *Users) GetUsers(ctx context.Context) ([]models.User, error) {
	const op = "usecase.Users.GetUsers"

	users, err := uc.usersRepo.GetUsers(ctx)
	if err != nil {
		return users, fmt.Errorf("%s: %w", op, err)
	}

	return users, nil
}
