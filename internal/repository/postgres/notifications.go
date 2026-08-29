package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
)

// NotificationsRepository stores in-app notifications and push tokens.
type NotificationsRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewNotifications(conn *sqlx.DB, c *trmsqlx.CtxGetter) *NotificationsRepository {
	return &NotificationsRepository{
		db:     conn,
		getter: c,
	}
}

const notificationColumns = "notification_id, user_id, event_id, type, mark_id, task_id, title, body, read_at, created_at"

// AddNotification inserts n and returns its id. A duplicate (event_id,
// user_id) pair is silently skipped: the returned id is 0 and created is
// false, which keeps event consumers idempotent.
func (r *NotificationsRepository) AddNotification(ctx context.Context, n models.Notification) (int64, bool, error) {
	const op = "storage.postgres.AddNotification"

	query := `
			INSERT INTO
				notifications (user_id, event_id, type, mark_id, task_id, title, body)
			VALUES
				($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (event_id, user_id) DO NOTHING
			RETURNING notification_id
			`
	var id int64
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	err := tr.GetContext(ctx, &id, query, n.UserID, n.EventID, n.Type, n.MarkID, n.TaskID, n.Title, n.Body)
	switch {
	case err == nil:
		return id, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	default:
		return 0, false, fmt.Errorf("%s: %w", op, err)
	}
}

// GetNotificationsByUserId returns a page of the user's notifications,
// newest first.
func (r *NotificationsRepository) GetNotificationsByUserId(ctx context.Context, userId int, filters models.GetNotificationsFilters) (models.Page[models.Notification], error) {
	const op = "storage.postgres.GetNotificationsByUserId"

	q := newListQuery(notificationColumns, "notifications").
		Where("user_id = ?", userId).
		OrderBy("created_at DESC, notification_id DESC").
		Paginate(filters.Pagination)

	if filters.UnreadOnly {
		q.Where("read_at IS NULL")
	}

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.Notification](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// CountUnreadByUserId returns the number of the user's unread notifications.
func (r *NotificationsRepository) CountUnreadByUserId(ctx context.Context, userId int) (int, error) {
	const op = "storage.postgres.CountUnreadByUserId"

	var count int
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	query := "SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL"
	if err := tr.GetContext(ctx, &count, query, userId); err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return count, nil
}

// MarkRead sets read_at on the user's notification. A notification of
// another user (or a missing one) yields repository.ErrNotFound; marking an
// already read notification is a no-op.
func (r *NotificationsRepository) MarkRead(ctx context.Context, userId int, id int) error {
	const op = "storage.postgres.MarkRead"

	query := `
			UPDATE notifications
			SET read_at = COALESCE(read_at, NOW())
			WHERE notification_id = $1 AND user_id = $2
			`
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, id, userId)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if affected == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// MarkAllRead sets read_at on every unread notification of the user and
// returns how many were updated.
func (r *NotificationsRepository) MarkAllRead(ctx context.Context, userId int) (int64, error) {
	const op = "storage.postgres.MarkAllRead"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, "UPDATE notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL", userId)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return affected, nil
}

// UpsertDevice registers a push token for the user. A token already
// registered (by this or another user) is moved to the user with the new
// platform.
func (r *NotificationsRepository) UpsertDevice(ctx context.Context, device models.UserDevice) (models.UserDevice, error) {
	const op = "storage.postgres.UpsertDevice"

	query := `
			INSERT INTO
				user_devices (user_id, platform, token)
			VALUES
				($1, $2, $3)
			ON CONFLICT (token) DO UPDATE
				SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform
			RETURNING device_id, user_id, platform, token, created_at
			`
	var out models.UserDevice
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &out, query, device.UserID, device.Platform, device.Token); err != nil {
		return out, fmt.Errorf("%s: %w", op, err)
	}

	return out, nil
}

// DeleteDevice removes the user's push token; a token the user does not own
// yields repository.ErrNotFound.
func (r *NotificationsRepository) DeleteDevice(ctx context.Context, userId int, token string) error {
	const op = "storage.postgres.DeleteDevice"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, "DELETE FROM user_devices WHERE user_id = $1 AND token = $2", userId, token)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if affected == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// GetDevicesByUserId lists the user's registered push tokens.
func (r *NotificationsRepository) GetDevicesByUserId(ctx context.Context, userId int) ([]models.UserDevice, error) {
	const op = "storage.postgres.GetDevicesByUserId"

	devices := make([]models.UserDevice, 0)
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	query := "SELECT device_id, user_id, platform, token, created_at FROM user_devices WHERE user_id = $1 ORDER BY device_id"
	if err := tr.SelectContext(ctx, &devices, query, userId); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return devices, nil
}
