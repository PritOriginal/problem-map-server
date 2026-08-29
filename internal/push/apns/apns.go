// Package apns is the placeholder for Apple Push Notification service.
//
// TODO(apns): implement token-based (JWT, .p8 key) delivery over HTTP/2 to
// api.push.apple.com (api.sandbox.push.apple.com when Sandbox is set):
//   - sign a JWT with ES256 by KeyID/TeamID and cache it for < 1h;
//   - POST /3/device/{token} with headers apns-topic (BundleID),
//     apns-push-type: alert, apns-priority: 10, authorization: bearer <jwt>;
//   - body {"aps": {"alert": {"title", "body"}}, ...push.Data(n)};
//   - map 410 Unregistered / 400 BadDeviceToken to push.ErrInvalidToken,
//     retry 5xx/429 like fcm does.
//
// Until then Sender logs the notification and returns push.ErrNotImplemented,
// which the use case counts as "unsupported" and does not treat as a failure.
package apns

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/push"
)

// Sender is the APNs stub; see the package comment.
type Sender struct {
	log      *slog.Logger
	cfg      config.APNsConfig
	warnOnce sync.Once
}

// New builds the stub. The config is kept so the real implementation can
// pick it up without changing the wiring in internal/app/notifier.
func New(log *slog.Logger, cfg config.APNsConfig) *Sender {
	return &Sender{log: log.With(slog.String("component", "push.apns")), cfg: cfg}
}

// Send logs the notification and returns push.ErrNotImplemented.
func (s *Sender) Send(_ context.Context, device models.UserDevice, n models.Notification) error {
	s.warnOnce.Do(func() {
		s.log.Warn("APNs sender is not implemented: ios pushes are only logged",
			slog.String("bundle_id", s.cfg.BundleID), slog.Bool("sandbox", s.cfg.Sandbox))
	})
	s.log.Debug("push notification (apns stub)",
		slog.Int("user_id", n.UserID),
		slog.String("type", string(n.Type)),
		slog.String("title", n.Title),
		slog.String("token", device.Token),
	)
	return fmt.Errorf("apns: %w", push.ErrNotImplemented)
}
