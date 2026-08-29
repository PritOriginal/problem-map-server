package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

// SyncTasksRepository is the part of the tasks storage the sync needs.
type SyncTasksRepository interface {
	GetTasksByUserId(ctx context.Context, userId int, filters models.GetTasksByUserIdFilters) (models.Page[models.Task], error)
}

// SyncNotificationsRepository is the part of the notifications storage the
// sync needs.
type SyncNotificationsRepository interface {
	GetNotificationsByUserId(ctx context.Context, userId int, filters models.GetNotificationsFilters) (models.Page[models.Notification], error)
}

// SyncChecksRepository is the part of the checks storage the sync needs.
type SyncChecksRepository interface {
	GetChecksByUserIdSince(ctx context.Context, userId int, since time.Time, p models.Pagination) (models.Page[models.Check], error)
}

type SyncRepositories struct {
	Tasks         SyncTasksRepository
	Notifications SyncNotificationsRepository
	Checks        SyncChecksRepository
}

// Sync gathers a user's personal changes after an instant in one call, for
// mobile clients coming back online (GET /users/me/sync).
type Sync struct {
	log   *slog.Logger
	repos SyncRepositories
}

func NewSync(log *slog.Logger, repos SyncRepositories) *Sync {
	return &Sync{log: log, repos: repos}
}

// GetUserSync returns the user's tasks updated after filters.Since, the
// unread notifications created after it and the checks the user submitted
// after it. ServerTime is taken before the queries, so using it as the
// next Since cannot skip a concurrent change.
func (uc *Sync) GetUserSync(ctx context.Context, userId int, filters models.UserSyncFilters) (models.UserSync, error) {
	const op = "usecase.Sync.GetUserSync"

	if err := filters.Validate(); err != nil {
		return models.UserSync{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	sync := models.UserSync{ServerTime: time.Now().UTC()}

	tasks, err := uc.repos.Tasks.GetTasksByUserId(ctx, userId, models.GetTasksByUserIdFilters{
		UpdatedSince: filters.Since,
		Pagination:   filters.Pagination,
	})
	if err != nil {
		return models.UserSync{}, mapRepoErr(op, err)
	}
	sync.Tasks, sync.Totals.Tasks = tasks.Items, tasks.Total

	notifications, err := uc.repos.Notifications.GetNotificationsByUserId(ctx, userId, models.GetNotificationsFilters{
		UnreadOnly:   true,
		CreatedSince: filters.Since,
		Pagination:   filters.Pagination,
	})
	if err != nil {
		return models.UserSync{}, mapRepoErr(op, err)
	}
	sync.Notifications, sync.Totals.Notifications = notifications.Items, notifications.Total

	checks, err := uc.repos.Checks.GetChecksByUserIdSince(ctx, userId, filters.Since, filters.Pagination)
	if err != nil {
		return models.UserSync{}, mapRepoErr(op, err)
	}
	sync.Checks, sync.Totals.Checks = checks.Items, checks.Total

	return sync, nil
}
