package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
)

// RefreshStore keeps the ids (jti) of the refresh tokens that are still
// valid. A refresh token is accepted only while its id is in the store, and
// the id is deleted the moment the token is used, which makes the rotation
// one-time.
type RefreshStore interface {
	// SaveRefresh registers the token id; the entry expires after ttl.
	SaveRefresh(ctx context.Context, userID int, jti string, ttl time.Duration) error
	// DeleteRefresh removes the token id and reports whether it existed.
	DeleteRefresh(ctx context.Context, userID int, jti string) (bool, error)
	// DeleteAllRefresh removes every token id of the user.
	DeleteAllRefresh(ctx context.Context, userID int) error
}

// AuthVersionStore keeps the per-user auth version. The version is put into
// every token; bumping it invalidates all tokens issued before, which is how
// a role change, a password change or "logout everywhere" takes effect
// before the access token expires.
type AuthVersionStore interface {
	// AuthVersion returns the current version; a user without one is at 0.
	AuthVersion(ctx context.Context, userID int) (int64, error)
	// IncrAuthVersion bumps the version and returns the new value.
	IncrAuthVersion(ctx context.Context, userID int) (int64, error)
}

// sessions bundles the optional Redis-backed stores shared by the auth and
// users usecases. Both stores may be nil (no Redis configured) and every
// operation fails open: an unavailable store is logged at warn level and
// the request continues as if the check passed. This keeps sign-in and
// refresh working during a Redis outage at the cost of revocation and role
// freshness being delayed until Redis is back (see README, "Аутентификация").
type sessions struct {
	log      *slog.Logger
	refresh  RefreshStore
	versions AuthVersionStore
}

// version returns the user's current auth version, or 0 when the store is
// unavailable.
func (s sessions) version(ctx context.Context, op string, userID int) int64 {
	if s.versions == nil {
		return 0
	}
	v, err := s.versions.AuthVersion(ctx, userID)
	if err != nil {
		s.log.Warn("auth version store unavailable, failing open",
			slog.String("op", op), slog.Int("user_id", userID), logger.Err(err))
		return 0
	}
	return v
}

// revokeAll invalidates every session of the user: all refresh tokens are
// deleted and the auth version is bumped so that access tokens stop
// passing the middleware. Failures are logged and swallowed (fail open).
func (s sessions) revokeAll(ctx context.Context, op string, userID int) {
	if s.refresh != nil {
		if err := s.refresh.DeleteAllRefresh(ctx, userID); err != nil {
			s.log.Warn("failed to revoke refresh tokens",
				slog.String("op", op), slog.Int("user_id", userID), logger.Err(err))
		}
	}
	if s.versions != nil {
		if _, err := s.versions.IncrAuthVersion(ctx, userID); err != nil {
			s.log.Warn("failed to bump auth version",
				slog.String("op", op), slog.Int("user_id", userID), logger.Err(err))
		}
	}
}
