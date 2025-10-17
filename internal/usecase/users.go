package usecase

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage/postgres"
)

type Users struct {
	usersRepo postgres.UsersRepository
}

func NewUsers(usersRepo postgres.UsersRepository) *Users {
	return &Users{usersRepo: usersRepo}
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
