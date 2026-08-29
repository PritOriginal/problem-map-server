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
	// AddRatingEvent stores the event and applies its delta to users.rating.
	AddRatingEvent(ctx context.Context, event models.RatingEvent) (int64, error)
	GetRatingEvents(ctx context.Context, userId int, p models.Pagination) (models.Page[models.RatingEvent], error)
	GetLeaderboard(ctx context.Context, p models.Pagination) (models.Page[models.User], error)
	GetUserStats(ctx context.Context, userId int) (models.UserStats, error)
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
func (uc *Users) ListLeaderboard(ctx context.Context, p models.Pagination) (models.Page[models.User], error) {
	const op = "usecase.Users.ListLeaderboard"

	if err := p.Validate(); err != nil {
		return models.Page[models.User]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Users.GetLeaderboard(ctx, p)
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
