package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/google/uuid"
)

type NotificationsRepository interface {
	// AddNotification inserts n; created is false when the same
	// (event_id, user_id) pair already exists.
	AddNotification(ctx context.Context, n models.Notification) (id int64, created bool, err error)
	GetNotificationsByUserId(ctx context.Context, userId int, filters models.GetNotificationsFilters) (models.Page[models.Notification], error)
	CountUnreadByUserId(ctx context.Context, userId int) (int, error)
	MarkRead(ctx context.Context, userId int, id int) error
	MarkAllRead(ctx context.Context, userId int) (int64, error)
}

type DevicesRepository interface {
	UpsertDevice(ctx context.Context, device models.UserDevice) (models.UserDevice, error)
	DeleteDevice(ctx context.Context, userId int, token string) error
	GetDevicesByUserId(ctx context.Context, userId int) ([]models.UserDevice, error)
}

// PushSender delivers a notification to the user's devices. This is the
// extension point for a real push provider (FCM/APNs): implement it, wire
// the implementation in internal/app/notifier instead of LogPushSender.
type PushSender interface {
	Send(ctx context.Context, devices []models.UserDevice, n models.Notification) error
}

// LogPushSender is the default PushSender: it only logs what would be sent.
type LogPushSender struct {
	log *slog.Logger
}

func NewLogPushSender(log *slog.Logger) *LogPushSender {
	return &LogPushSender{log: log.With(slog.String("component", "push"))}
}

func (s *LogPushSender) Send(_ context.Context, devices []models.UserDevice, n models.Notification) error {
	tokens := make([]string, 0, len(devices))
	for _, d := range devices {
		tokens = append(tokens, string(d.Platform)+":"+d.Token)
	}
	s.log.Info("push notification (log sender)",
		slog.Int("user_id", n.UserID),
		slog.String("type", string(n.Type)),
		slog.String("title", n.Title),
		slog.Any("devices", tokens),
	)
	return nil
}

type NotificationsRepositories struct {
	Notifications NotificationsRepository
	Devices       DevicesRepository
}

// Notifications manages a user's in-app notifications and push tokens.
type Notifications struct {
	log   *slog.Logger
	repos NotificationsRepositories
	push  PushSender
}

// NewNotifications builds the use case; a nil push disables push delivery.
func NewNotifications(log *slog.Logger, push PushSender, repos NotificationsRepositories) *Notifications {
	return &Notifications{
		log:   log,
		repos: repos,
		push:  push,
	}
}

// Create stores n and, when it is new, pushes it to the user's devices. A
// missing EventID is generated, so manual notifications are never
// deduplicated. Push failures are logged, not returned: the in-app
// notification is already persisted.
func (uc *Notifications) Create(ctx context.Context, n models.Notification) (int64, bool, error) {
	const op = "usecase.Notifications.Create"

	if n.EventID == "" {
		n.EventID = uuid.NewString()
	}

	id, created, err := uc.repos.Notifications.AddNotification(ctx, n)
	if err != nil {
		return 0, false, mapRepoErr(op, err)
	}
	if !created {
		uc.log.Debug("duplicate notification skipped",
			slog.String("event_id", n.EventID), slog.Int("user_id", n.UserID))
		return 0, false, nil
	}
	n.ID = int(id)

	if uc.push != nil {
		devices, err := uc.repos.Devices.GetDevicesByUserId(ctx, n.UserID)
		if err != nil {
			uc.log.Warn("failed to load user devices, push skipped", slog.String("op", op), slogger.Err(err))
			return id, true, nil
		}
		if len(devices) > 0 {
			if err := uc.push.Send(ctx, devices, n); err != nil {
				uc.log.Warn("failed to send push", slog.String("op", op), slogger.Err(err))
			}
		}
	}

	return id, true, nil
}

// List returns a page of the user's notifications, newest first.
func (uc *Notifications) List(ctx context.Context, userId int, filters models.GetNotificationsFilters) (models.Page[models.Notification], error) {
	const op = "usecase.Notifications.List"

	if err := filters.Validate(); err != nil {
		return models.Page[models.Notification]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	page, err := uc.repos.Notifications.GetNotificationsByUserId(ctx, userId, filters)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	return page, nil
}

// UnreadCount returns the number of the user's unread notifications.
func (uc *Notifications) UnreadCount(ctx context.Context, userId int) (int, error) {
	const op = "usecase.Notifications.UnreadCount"

	count, err := uc.repos.Notifications.CountUnreadByUserId(ctx, userId)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	return count, nil
}

// MarkRead marks one of the user's notifications as read.
func (uc *Notifications) MarkRead(ctx context.Context, userId int, id int) error {
	const op = "usecase.Notifications.MarkRead"

	if err := uc.repos.Notifications.MarkRead(ctx, userId, id); err != nil {
		return mapRepoErr(op, err)
	}

	return nil
}

// MarkAllRead marks every unread notification of the user as read and
// returns how many were updated.
func (uc *Notifications) MarkAllRead(ctx context.Context, userId int) (int64, error) {
	const op = "usecase.Notifications.MarkAllRead"

	n, err := uc.repos.Notifications.MarkAllRead(ctx, userId)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	return n, nil
}

// RegisterDevice upserts a push token for the user.
func (uc *Notifications) RegisterDevice(ctx context.Context, device models.UserDevice) (models.UserDevice, error) {
	const op = "usecase.Notifications.RegisterDevice"

	switch device.Platform {
	case models.PlatformAndroid, models.PlatformIOS, models.PlatformWeb:
	default:
		return models.UserDevice{}, fmt.Errorf("%s: %w: unknown platform %q", op, ErrInvalidArgument, device.Platform)
	}
	if device.Token == "" {
		return models.UserDevice{}, fmt.Errorf("%s: %w: empty token", op, ErrInvalidArgument)
	}

	out, err := uc.repos.Devices.UpsertDevice(ctx, device)
	if err != nil {
		return out, mapRepoErr(op, err)
	}

	return out, nil
}

// DeleteDevice removes the user's push token.
func (uc *Notifications) DeleteDevice(ctx context.Context, userId int, token string) error {
	const op = "usecase.Notifications.DeleteDevice"

	if err := uc.repos.Devices.DeleteDevice(ctx, userId, token); err != nil {
		return mapRepoErr(op, err)
	}

	return nil
}
