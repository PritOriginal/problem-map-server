package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	passwordUtils "github.com/PritOriginal/problem-map-server/pkg/password"
)

// MinPasswordLength is the shortest password accepted on sign-up and change.
const MinPasswordLength = 8

type UsersRepository interface {
	GetUserById(ctx context.Context, id int) (models.User, error)
	GetUserByLogin(ctx context.Context, username string) (models.User, error)
	GetUsers(ctx context.Context, p models.Pagination) (models.Page[models.User], error)
	AddUser(ctx context.Context, user models.User) (int64, error)
	UpdatePassword(ctx context.Context, id int, passwordHash string) error
	UpdateRole(ctx context.Context, id int, role models.Role) error
	CountByRole(ctx context.Context, role models.Role) (int64, error)
	// AddRatingEvent stores the event and applies its delta to users.rating.
	AddRatingEvent(ctx context.Context, event models.RatingEvent) (int64, error)
	GetRatingEvents(ctx context.Context, userId int, p models.Pagination) (models.Page[models.RatingEvent], error)
	GetLeaderboard(ctx context.Context, f models.LeaderboardFilters, p models.Pagination) (models.Page[models.LeaderboardEntry], error)
	GetUserStats(ctx context.Context, userId int) (models.UserStats, error)
}

type Users struct {
	log      *slog.Logger
	repos    UsersRepositories
	sessions sessions
}

// UsersRepositories are the users dependencies. RefreshTokens and
// AuthVersions are optional (see AuthRepositories); without them a password
// or role change does not invalidate the tokens already issued.
type UsersRepositories struct {
	Users         UsersRepository
	RefreshTokens RefreshStore
	AuthVersions  AuthVersionStore
}

func NewUsers(log *slog.Logger, repos UsersRepositories) *Users {
	return &Users{
		log:      log,
		repos:    repos,
		sessions: sessions{log: log, refresh: repos.RefreshTokens, versions: repos.AuthVersions},
	}
}

func (uc *Users) GetUserById(ctx context.Context, id int) (models.User, error) {
	const op = "usecase.Users.GetUserById"

	user, err := uc.repos.Users.GetUserById(ctx, id)
	if err != nil {
		return user, mapRepoErr(op, err)
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
		return page, mapRepoErr(op, err)
	}

	return page, nil
}

// GetUsers returns all users without pagination (gRPC, tasker).
func (uc *Users) GetUsers(ctx context.Context) ([]models.User, error) {
	const op = "usecase.Users.GetUsers"

	page, err := uc.repos.Users.GetUsers(ctx, models.Pagination{})
	if err != nil {
		return page.Items, mapRepoErr(op, err)
	}

	return page.Items, nil
}

// ChangePassword replaces the user's password after verifying the old one
// and revokes every session of the user (refresh tokens and auth version).
// A wrong old password yields ErrForbidden, a too short new password
// ErrInvalidArgument.
func (uc *Users) ChangePassword(ctx context.Context, id int, oldPassword, newPassword string) error {
	const op = "usecase.Users.ChangePassword"

	if len(newPassword) < MinPasswordLength {
		return fmt.Errorf("%s: %w: password shorter than %d", op, ErrInvalidArgument, MinPasswordLength)
	}

	user, err := uc.repos.Users.GetUserById(ctx, id)
	if err != nil {
		return mapRepoErr(op, err)
	}

	if !passwordUtils.CheckPasswordHash(oldPassword, user.PasswordHash) {
		return fmt.Errorf("%s: %w", op, ErrForbidden)
	}

	hash, err := passwordUtils.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := uc.repos.Users.UpdatePassword(ctx, id, hash); err != nil {
		return mapRepoErr(op, err)
	}

	uc.sessions.revokeAll(ctx, op, id)

	return nil
}

// SetRole changes the role of user id on behalf of actorID (an admin) and
// bumps the auth version so that tokens carrying the old role stop working.
// An admin may not take the admin role away from themselves while they are
// the only admin (ErrForbidden), so the system cannot end up without one.
// An unknown role yields ErrInvalidArgument, an unknown user ErrNotFound.
func (uc *Users) SetRole(ctx context.Context, actorID, id int, role models.Role) error {
	const op = "usecase.Users.SetRole"

	if !role.Valid() {
		return fmt.Errorf("%s: %w: unknown role %q", op, ErrInvalidArgument, role)
	}

	if actorID == id && role != models.RoleAdmin {
		admins, err := uc.repos.Users.CountByRole(ctx, models.RoleAdmin)
		if err != nil {
			return mapRepoErr(op, err)
		}
		if admins <= 1 {
			return fmt.Errorf("%s: %w: the last admin cannot give up the role", op, ErrForbidden)
		}
	}

	if err := uc.repos.Users.UpdateRole(ctx, id, role); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%s: %w", op, ErrNotFound)
		}
		return mapRepoErr(op, err)
	}

	uc.sessions.revokeAll(ctx, op, id)

	return nil
}

// GetUserStats returns the activity summary of a user.
func (uc *Users) GetUserStats(ctx context.Context, id int) (models.UserStats, error) {
	const op = "usecase.Users.GetUserStats"

	stats, err := uc.repos.Users.GetUserStats(ctx, id)
	if err != nil {
		return stats, mapRepoErr(op, err)
	}

	return stats, nil
}

// ListLeaderboard returns a page of users ordered by rating, highest first.
// The filters restrict the rating to the events inside an admin boundary
// and/or a period (see models.LeaderboardFilters); an unknown period is
// ErrInvalidArgument.
func (uc *Users) ListLeaderboard(ctx context.Context, f models.LeaderboardFilters, p models.Pagination) (models.Page[models.LeaderboardEntry], error) {
	const op = "usecase.Users.ListLeaderboard"

	if err := f.Period.Validate(); err != nil {
		return models.Page[models.LeaderboardEntry]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if err := p.Validate(); err != nil {
		return models.Page[models.LeaderboardEntry]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Users.GetLeaderboard(ctx, f, p)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	return page, nil
}

// Requester identifies the authenticated caller for authorization checks.
type Requester struct {
	ID   int
	Role models.Role
}

// CanAccessUser reports whether the requester may read private data of the
// given user: the owner and moderators/admins can.
func (r Requester) CanAccessUser(userId int) bool {
	return r.ID == userId || r.Role == models.RoleModerator || r.Role == models.RoleAdmin
}

// ListRatingEvents returns the rating history of a user, newest first. Only
// the owner or a moderator/admin may read it; others get ErrForbidden.
func (uc *Users) ListRatingEvents(ctx context.Context, requester Requester, userId int, p models.Pagination) (models.Page[models.RatingEvent], error) {
	const op = "usecase.Users.ListRatingEvents"

	if !requester.CanAccessUser(userId) {
		return models.Page[models.RatingEvent]{}, fmt.Errorf("%s: %w", op, ErrForbidden)
	}

	if err := p.Validate(); err != nil {
		return models.Page[models.RatingEvent]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Users.GetRatingEvents(ctx, userId, p)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	return page, nil
}
