package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	passwordUtils "github.com/PritOriginal/problem-map-server/pkg/password"
	"github.com/PritOriginal/problem-map-server/pkg/token"
)

type Auth struct {
	log       *slog.Logger
	usersRepo UsersRepository
	authCfg   config.AuthConfing
}

func NewAuth(log *slog.Logger, usersRepo UsersRepository, authCfg config.AuthConfing) *Auth {
	return &Auth{log: log, usersRepo: usersRepo, authCfg: authCfg}
}

func (uc *Auth) SignUp(ctx context.Context, name, username, password string) (int64, error) {
	const op = "usecase.Users.SignUp"

	passwordHash, err := passwordUtils.HashPassword(password)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	user := models.User{
		Name:         name,
		Username:     username,
		PasswordHash: passwordHash,
	}

	id, err := uc.usersRepo.AddUser(ctx, user)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (uc *Auth) SignIn(ctx context.Context, username, password string) (string, string, error) {
	const op = "usecase.Users.SignIn"

	user, err := uc.usersRepo.GetUserByUsername(ctx, username)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	if !passwordUtils.CheckPasswordHash(password, user.PasswordHash) {
		return "", "", fmt.Errorf("%s: %w", op, storage.ErrNotFound)
	}

	accessToken, refreshToken, err := uc.generateTokens(user.Id)

	return accessToken, refreshToken, nil
}

func (uc *Auth) RefreshTokens(ctx context.Context, refreshToken string) (string, string, error) {
	const op = "usecase.Users.RefreshTokens"

	sub, err := token.ValidateToken(refreshToken, uc.authCfg.JWT.Refresh.Key)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	userId, err := strconv.Atoi(fmt.Sprint(sub))
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	user, err := uc.usersRepo.GetUserById(ctx, userId)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	accessToken, refreshToken, err := uc.generateTokens(user.Id)

	return accessToken, refreshToken, nil
}

func (uc *Auth) generateTokens(userId int) (string, string, error) {
	const op = "usecase.Users.generateTokens"

	accessToken, err := token.CreateToken(uc.authCfg.JWT.Access.ExpiredIn, userId, uc.authCfg.JWT.Access.Key)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	refreshToken, err := token.CreateToken(uc.authCfg.JWT.Refresh.ExpiredIn, userId, uc.authCfg.JWT.Refresh.Key)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	return accessToken, refreshToken, nil
}
