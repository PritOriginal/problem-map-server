package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	passwordUtils "github.com/PritOriginal/problem-map-server/pkg/password"
	"github.com/PritOriginal/problem-map-server/pkg/token"
)

type Auth struct {
	log     *slog.Logger
	repos   AuthRepositories
	authCfg config.AuthConfing
}

type AuthRepositories struct {
	Users UsersRepository
}

func NewAuth(log *slog.Logger, authCfg config.AuthConfing, repos AuthRepositories) *Auth {
	return &Auth{log: log, repos: repos, authCfg: authCfg}
}

type SignUpParams struct {
	Username  string
	Login     string
	Password  string
	HomePoint *models.Point
}

func (uc *Auth) SignUp(ctx context.Context, params SignUpParams) (int64, error) {
	const op = "usecase.Users.SignUp"

	passwordHash, err := passwordUtils.HashPassword(params.Password)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	user := models.User{
		Name:         params.Username,
		Login:        params.Login,
		PasswordHash: passwordHash,
		HomePoint:    params.HomePoint,
	}

	_, err = uc.repos.Users.GetUserByLogin(ctx, user.Login)
	if err == nil {
		return 0, ErrConflict
	}
	if !errors.Is(err, repository.ErrNotFound) {
		uc.log.Debug("GetUserByLogin err", logger.Err(err))
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	id, err := uc.repos.Users.AddUser(ctx, user)
	if err != nil {
		return id, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}

func (uc *Auth) SignIn(ctx context.Context, login, password string) (string, string, error) {
	const op = "usecase.Users.SignIn"

	user, err := uc.repos.Users.GetUserByLogin(ctx, login)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	if !passwordUtils.CheckPasswordHash(password, user.PasswordHash) {
		return "", "", fmt.Errorf("%s: %w", op, repository.ErrNotFound)
	}

	accessToken, refreshToken, err := uc.generateTokens(user.Id)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	return accessToken, refreshToken, nil
}

func (uc *Auth) RefreshTokens(ctx context.Context, refreshToken string) (string, string, error) {
	const op = "usecase.Users.RefreshTokens"

	sub, err := token.ValidateToken(refreshToken, uc.authCfg.JWT.Refresh.Key)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, ErrUnauthorized)
	}

	userId, err := strconv.Atoi(fmt.Sprint(sub))
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, ErrUnauthorized)
	}

	user, err := uc.repos.Users.GetUserById(ctx, userId)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", "", fmt.Errorf("%s: %w", op, ErrUnauthorized)
		} else {
			return "", "", fmt.Errorf("%s: %w", op, err)
		}
	}

	accessToken, refreshToken, err := uc.generateTokens(user.Id)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

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
