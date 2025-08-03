package usecase

import (
	"context"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage/db"
)

type Users interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUsers(ctx context.Context) ([]models.User, error)
	AddUser(ctx context.Context, user models.User) (int64, error)
}

type UsersUseCase struct {
	usersRepo db.UsersRepository
}

func NewUsers(usersRepo db.UsersRepository) *UsersUseCase {
	return &UsersUseCase{usersRepo: usersRepo}
}

func (uc *UsersUseCase) GetUserById(ctx context.Context, id int) (models.User, error) {
	const op = "usecase.Users.GetUserById"

	user, err := uc.usersRepo.GetUserById(ctx, id)
	if err != nil {
		return user, fmt.Errorf("%s: %w", op, err)
	}

	return user, nil
}

func (uc *UsersUseCase) GetUsers(ctx context.Context) ([]models.User, error) {
	const op = "usecase.Users.GetUsers"

	users, err := uc.usersRepo.GetUsers(ctx)
	if err != nil {
		return users, fmt.Errorf("%s: %w", op, err)
	}

	return users, nil
}

func (uc *UsersUseCase) AddUser(ctx context.Context, user models.User) (int64, error) {
	const op = "usecase.Users.GetUserById"

	id, err := uc.usersRepo.AddUser(ctx, user)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}
