package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	passwordUtils "github.com/PritOriginal/problem-map-server/pkg/password"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/google/uuid"
)

type Auth struct {
	log      *slog.Logger
	repos    AuthRepositories
	authCfg  config.AuthConfig
	sessions sessions
}

// AuthRepositories are the auth dependencies. RefreshTokens and
// AuthVersions are optional: without them refresh tokens are not tracked
// (no rotation, no revocation) and every token is issued with version 0.
type AuthRepositories struct {
	Users         UsersRepository
	RefreshTokens RefreshStore
	AuthVersions  AuthVersionStore
}

func NewAuth(log *slog.Logger, authCfg config.AuthConfig, repos AuthRepositories) *Auth {
	return &Auth{
		log:      log,
		repos:    repos,
		authCfg:  authCfg,
		sessions: sessions{log: log, refresh: repos.RefreshTokens, versions: repos.AuthVersions},
	}
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
		Role:         models.RoleUser,
	}

	_, err = uc.repos.Users.GetUserByLogin(ctx, user.Login)
	if err == nil {
		return 0, fmt.Errorf("%s: %w", op, ErrConflict)
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return 0, mapRepoErr(op, err)
	}

	id, err := uc.repos.Users.AddUser(ctx, user)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	return id, nil
}

func (uc *Auth) SignIn(ctx context.Context, login, password string) (string, string, error) {
	const op = "usecase.Users.SignIn"

	user, err := uc.repos.Users.GetUserByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", "", fmt.Errorf("%s: %w", op, ErrUnauthorized)
		}
		return "", "", mapRepoErr(op, err)
	}

	if !passwordUtils.CheckPasswordHash(password, user.PasswordHash) {
		return "", "", fmt.Errorf("%s: %w", op, ErrUnauthorized)
	}

	accessToken, refreshToken, err := uc.generateTokens(ctx, user)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	return accessToken, refreshToken, nil
}

// RefreshTokens exchanges a refresh token for a new token pair. The used
// token is invalidated (one-time rotation). Presenting a token that was
// already used or revoked is treated as a sign of theft: every refresh
// token of the user is revoked and ErrUnauthorized is returned.
func (uc *Auth) RefreshTokens(ctx context.Context, refreshToken string) (string, string, error) {
	const op = "usecase.Users.RefreshTokens"

	claims, err := uc.parseRefresh(refreshToken)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, ErrUnauthorized)
	}

	if uc.repos.RefreshTokens != nil {
		ok, err := uc.repos.RefreshTokens.DeleteRefresh(ctx, claims.UserID, claims.ID)
		switch {
		case err != nil:
			uc.log.Warn("refresh store unavailable, failing open",
				slog.String("op", op), slog.Int("user_id", claims.UserID), logger.Err(err))
		case !ok:
			uc.log.Warn("refresh token reuse detected, revoking all sessions",
				slog.String("op", op), slog.Int("user_id", claims.UserID))
			if err := uc.repos.RefreshTokens.DeleteAllRefresh(ctx, claims.UserID); err != nil {
				uc.log.Warn("failed to revoke refresh tokens",
					slog.String("op", op), slog.Int("user_id", claims.UserID), logger.Err(err))
			}
			return "", "", fmt.Errorf("%s: %w", op, ErrUnauthorized)
		}
	}

	user, err := uc.repos.Users.GetUserById(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", "", fmt.Errorf("%s: %w", op, ErrUnauthorized)
		}
		return "", "", mapRepoErr(op, err)
	}

	accessToken, newRefreshToken, err := uc.generateTokens(ctx, user)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	return accessToken, newRefreshToken, nil
}

// Logout revokes the given refresh token of the user. The token must belong
// to userID (the authenticated caller); otherwise ErrUnauthorized is
// returned. The access token stays valid until it expires.
func (uc *Auth) Logout(ctx context.Context, userID int, refreshToken string) error {
	const op = "usecase.Users.Logout"

	claims, err := uc.parseRefresh(refreshToken)
	if err != nil || claims.UserID != userID {
		return fmt.Errorf("%s: %w", op, ErrUnauthorized)
	}

	if uc.repos.RefreshTokens == nil {
		return nil
	}
	if _, err := uc.repos.RefreshTokens.DeleteRefresh(ctx, userID, claims.ID); err != nil {
		uc.log.Warn("refresh store unavailable, token not revoked",
			slog.String("op", op), slog.Int("user_id", userID), logger.Err(err))
	}

	return nil
}

// LogoutAll revokes every refresh token of the user and bumps the auth
// version, so that all existing access tokens are rejected as well.
func (uc *Auth) LogoutAll(ctx context.Context, userID int) error {
	const op = "usecase.Users.LogoutAll"

	uc.sessions.revokeAll(ctx, op, userID)

	return nil
}

// parseRefresh validates the signature of a refresh token and checks that it
// is a refresh token carrying an id.
func (uc *Auth) parseRefresh(refreshToken string) (token.Claims, error) {
	claims, err := token.ValidateClaims(refreshToken, uc.authCfg.JWT.Refresh.Key)
	if err != nil {
		return token.Claims{}, err
	}
	if claims.Type != token.TypeRefresh {
		return token.Claims{}, fmt.Errorf("unexpected token type %q", claims.Type)
	}
	if claims.ID == "" {
		return token.Claims{}, errors.New("missing token id")
	}

	return claims, nil
}

func (uc *Auth) generateTokens(ctx context.Context, user models.User) (string, string, error) {
	const op = "usecase.Users.generateTokens"

	role := user.Role
	if role == "" {
		role = models.RoleUser
	}
	version := uc.sessions.version(ctx, op, user.Id)

	accessToken, err := token.Create(token.Params{
		TTL:     uc.authCfg.JWT.Access.ExpiredIn,
		UserID:  user.Id,
		Role:    string(role),
		Type:    token.TypeAccess,
		Version: version,
	}, uc.authCfg.JWT.Access.Key)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	jti := uuid.NewString()
	refreshToken, err := token.Create(token.Params{
		TTL:     uc.authCfg.JWT.Refresh.ExpiredIn,
		UserID:  user.Id,
		Role:    string(role),
		Type:    token.TypeRefresh,
		Version: version,
		ID:      jti,
	}, uc.authCfg.JWT.Refresh.Key)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", op, err)
	}

	if uc.repos.RefreshTokens != nil {
		if err := uc.repos.RefreshTokens.SaveRefresh(ctx, user.Id, jti, uc.authCfg.JWT.Refresh.ExpiredIn); err != nil {
			uc.log.Warn("refresh store unavailable, token issued untracked",
				slog.String("op", op), slog.Int("user_id", user.Id), logger.Err(err))
		}
	}

	return accessToken, refreshToken, nil
}
