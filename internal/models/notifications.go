package models

import (
	"time"

	"github.com/guregu/null/v6"
)

// NotificationType identifies what a notification is about; it matches the
// domain event that produced it.
type NotificationType string

const (
	NotificationMarkStatusChanged NotificationType = "mark.status_changed"
	NotificationTaskAssigned      NotificationType = "task.assigned"
	NotificationCheckAdded        NotificationType = "check.added"
)

// Notification is an in-app message addressed to one user. EventID makes
// creation idempotent: the same event never produces two notifications for
// the same user (UNIQUE (event_id, user_id)).
type Notification struct {
	ID        int              `json:"id" db:"notification_id"`
	UserID    int              `json:"user_id" db:"user_id"`
	EventID   string           `json:"-" db:"event_id"`
	Type      NotificationType `json:"type" db:"type"`
	MarkID    null.Int         `json:"mark_id" db:"mark_id" swaggertype:"integer"`
	TaskID    null.Int         `json:"task_id" db:"task_id" swaggertype:"integer"`
	Title     string           `json:"title" db:"title"`
	Body      string           `json:"body" db:"body"`
	ReadAt    null.Time        `json:"read_at" db:"read_at" swaggertype:"string" format:"date-time"`
	CreatedAt time.Time        `json:"created_at" db:"created_at"`
}

// GetNotificationsFilters selects a page of a user's notifications.
type GetNotificationsFilters struct {
	// UnreadOnly keeps only notifications without read_at.
	UnreadOnly bool
	Pagination
}

// DevicePlatform is the push platform of a registered device.
type DevicePlatform string

const (
	PlatformAndroid DevicePlatform = "android"
	PlatformIOS     DevicePlatform = "ios"
	PlatformWeb     DevicePlatform = "web"
)

// UserDevice is a push token registered by a user's client. Token is unique
// across users: re-registering a token moves it to the new user.
type UserDevice struct {
	ID        int            `json:"id" db:"device_id"`
	UserID    int            `json:"user_id" db:"user_id"`
	Platform  DevicePlatform `json:"platform" db:"platform"`
	Token     string         `json:"token" db:"token"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
}
