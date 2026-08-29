package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/push"
	"github.com/PritOriginal/problem-map-server/internal/repository"
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

// PushSender delivers a notification to one device (see internal/push):
// push.ErrInvalidToken makes the use case drop the device,
// push.ErrNotImplemented is counted as unsupported, anything else is a
// delivery failure.
type PushSender interface {
	Send(ctx context.Context, device models.UserDevice, n models.Notification) error
}

// PushMetrics records the outcome of every push attempt
// (push_sent_total{platform,result}).
type PushMetrics interface {
	PushSent(platform models.DevicePlatform, result string)
}

// DefaultPushTimeout bounds the delivery of one notification to all devices
// of its addressee when no timeout is configured.
const DefaultPushTimeout = 15 * time.Second

type NotificationsRepositories struct {
	Notifications NotificationsRepository
	Devices       DevicesRepository
}

// NotificationsOption tunes push delivery of Notifications.
type NotificationsOption func(*Notifications)

// WithPushMetrics records push outcomes on m.
func WithPushMetrics(m PushMetrics) NotificationsOption {
	return func(uc *Notifications) { uc.metrics = m }
}

// WithPushTimeout bounds the push delivery of one notification; a
// non-positive d keeps DefaultPushTimeout.
func WithPushTimeout(d time.Duration) NotificationsOption {
	return func(uc *Notifications) {
		if d > 0 {
			uc.pushTimeout = d
		}
	}
}

// Notifications manages a user's in-app notifications and push tokens.
type Notifications struct {
	log         *slog.Logger
	repos       NotificationsRepositories
	push        PushSender
	metrics     PushMetrics
	pushTimeout time.Duration
}

// NewNotifications builds the use case; a nil push disables push delivery.
func NewNotifications(log *slog.Logger, push PushSender, repos NotificationsRepositories, opts ...NotificationsOption) *Notifications {
	uc := &Notifications{
		log:         log,
		repos:       repos,
		push:        push,
		metrics:     noopPushMetrics{},
		pushTimeout: DefaultPushTimeout,
	}
	for _, opt := range opts {
		opt(uc)
	}
	return uc
}

type noopPushMetrics struct{}

func (noopPushMetrics) PushSent(models.DevicePlatform, string) {}

// Create stores n and, when it is new, pushes it to the user's devices. A
// missing EventID is generated, so manual notifications are never
// deduplicated. Push failures are logged and counted, not returned: the
// in-app notification is already persisted.
//
// Delivery is synchronous, bounded by the push timeout: Create runs in the
// notifier worker where latency is not user-facing, the NATS queue group
// gives natural back-pressure and scale-out, and a finished Create means
// every push either reached the provider or was reported. An asynchronous
// pool would have to survive shutdown and detach from the handler's context
// to give the same guarantee.
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
		uc.sendPush(ctx, devices, n)
	}

	return id, true, nil
}

// sendPush delivers n to every device concurrently (the provider bounds the
// parallelism) and drops the devices whose tokens are dead.
func (uc *Notifications) sendPush(ctx context.Context, devices []models.UserDevice, n models.Notification) {
	if len(devices) == 0 {
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, uc.pushTimeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, device := range devices {
		wg.Go(func() {
			uc.sendPushToDevice(sendCtx, device, n)
		})
	}
	wg.Wait()
}

func (uc *Notifications) sendPushToDevice(ctx context.Context, device models.UserDevice, n models.Notification) {
	const op = "usecase.Notifications.sendPush"

	log := uc.log.With(slog.String("op", op), slog.Int("user_id", n.UserID),
		slog.Int("notification_id", n.ID), slog.String("platform", string(device.Platform)))

	err := uc.push.Send(ctx, device, n)
	switch {
	case err == nil:
		uc.metrics.PushSent(device.Platform, push.ResultOK)
	case errors.Is(err, push.ErrInvalidToken):
		uc.metrics.PushSent(device.Platform, push.ResultInvalidToken)
		log.Info("device token rejected by the provider, deleting device",
			slog.Int("device_id", device.ID), slogger.Err(err))
		// The device is deleted on the parent context: a push timeout must not
		// leave a dead token behind. A token re-registered to another user
		// meanwhile is not ours to delete (ErrNotFound).
		if err := uc.repos.Devices.DeleteDevice(context.WithoutCancel(ctx), device.UserID, device.Token); err != nil &&
			!errors.Is(err, repository.ErrNotFound) {
			log.Warn("failed to delete device with invalid token", slogger.Err(err))
		}
	case errors.Is(err, push.ErrNotImplemented):
		uc.metrics.PushSent(device.Platform, push.ResultUnsupported)
		log.Debug("push skipped: platform not supported", slogger.Err(err))
	default:
		uc.metrics.PushSent(device.Platform, push.ResultError)
		log.Warn("failed to send push", slogger.Err(err))
	}
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

// RegisterDevice upserts a push token for the user. A token is bound to a
// physical device, so one already registered by another account (a
// re-login on the same device) is moved to the caller: the caller proves
// possession of the token, and the previous owner simply stops receiving
// pushes on that device. UserID must come from the verified claims.
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
