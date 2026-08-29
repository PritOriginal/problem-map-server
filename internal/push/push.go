// Package push delivers notifications to user devices. Sender is the
// per-device contract every provider implements (fcm, apns); Multi routes a
// device to the provider of its platform and LogSender is the fallback that
// only logs.
package push

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/PritOriginal/problem-map-server/internal/models"
)

var (
	// ErrInvalidToken reports that the provider rejected the device token
	// for good (unregistered, malformed): the device should be deleted.
	ErrInvalidToken = errors.New("push: invalid device token")
	// ErrNotImplemented reports that the platform has no real provider yet.
	ErrNotImplemented = errors.New("push: provider not implemented")
)

// Sender delivers n to a single device.
type Sender interface {
	Send(ctx context.Context, device models.UserDevice, n models.Notification) error
}

// Data is the key/value payload attached to every push so the client can
// open the related screen.
func Data(n models.Notification) map[string]string {
	data := map[string]string{
		"type":            string(n.Type),
		"notification_id": strconv.Itoa(n.ID),
	}
	if n.MarkID.Valid {
		data["mark_id"] = strconv.FormatInt(n.MarkID.Int64, 10)
	}
	if n.TaskID.Valid {
		data["task_id"] = strconv.FormatInt(n.TaskID.Int64, 10)
	}
	return data
}

// MaskToken shortens a device token for logs: a token is a credential of the
// device, so only enough of it to correlate log lines is kept.
func MaskToken(token string) string {
	const keep = 6
	if len(token) <= keep*2 {
		return "***"
	}
	return token[:keep] + "..." + token[len(token)-keep:]
}

// LogSender is the fallback Sender: it only logs what would be sent.
type LogSender struct {
	log *slog.Logger
}

func NewLogSender(log *slog.Logger) *LogSender {
	return &LogSender{log: log.With(slog.String("component", "push"))}
}

func (s *LogSender) Send(_ context.Context, device models.UserDevice, n models.Notification) error {
	s.log.Info("push notification (log sender)",
		slog.Int("user_id", n.UserID),
		slog.String("type", string(n.Type)),
		slog.String("title", n.Title),
		slog.String("platform", string(device.Platform)),
		slog.String("token", MaskToken(device.Token)),
	)
	return nil
}

// Multi routes a device to the Sender of its platform; a platform without a
// sender goes to the fallback (a nil fallback yields ErrNotImplemented).
type Multi struct {
	senders  map[models.DevicePlatform]Sender
	fallback Sender
}

// NewMulti builds the router; nil senders are treated as absent.
func NewMulti(fallback Sender, senders map[models.DevicePlatform]Sender) *Multi {
	m := &Multi{senders: make(map[models.DevicePlatform]Sender, len(senders)), fallback: fallback}
	for platform, s := range senders {
		if s != nil {
			m.senders[platform] = s
		}
	}
	return m
}

func (m *Multi) Send(ctx context.Context, device models.UserDevice, n models.Notification) error {
	if s, ok := m.senders[device.Platform]; ok {
		return s.Send(ctx, device, n)
	}
	if m.fallback != nil {
		return m.fallback.Send(ctx, device, n)
	}
	return fmt.Errorf("%w: platform %q", ErrNotImplemented, device.Platform)
}
