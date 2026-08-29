// Package auth holds the token checks shared by the REST middleware and the
// gRPC interceptor, so that both transports accept exactly the same bearer
// tokens: the transport verifies the signature and the time claims, then
// hands the parsed claims to Verify.
package auth

import (
	"context"
	"errors"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/token"
)

// Rejection reasons returned by Verify. Their messages are safe to show to
// the client.
var (
	// ErrNotAccessToken is returned for tokens whose "typ" is not "access"
	// (refresh tokens and legacy tokens without the claim).
	ErrNotAccessToken = errors.New("not an access token")
	// ErrRevoked is returned when the "ver" claim differs from the version
	// reported by the VersionChecker: the user changed the password, was
	// assigned another role or logged out everywhere.
	ErrRevoked = errors.New("token revoked")
)

// VersionChecker returns the current auth version of a user. It is
// implemented by the Redis repository and by middleware.VersionCache.
type VersionChecker interface {
	AuthVersion(ctx context.Context, userID int) (int64, error)
}

// Identity is the authenticated caller derived from accepted claims.
type Identity struct {
	UserID int
	// Role defaults to models.RoleUser when the token carries no role.
	Role models.Role
}

// Verify applies the checks common to every transport to claims whose
// signature and expiry were already verified:
//   - the token must be an access token (ErrNotAccessToken);
//   - when versions is not nil, the "ver" claim must equal the stored
//     version (ErrRevoked). A lookup error fails open: it is logged at warn
//     level and the token is accepted, in line with the cache and the rate
//     limiter degrading softly without Redis.
//
// Pass a nil interface (not a typed nil pointer) to skip the version check.
func Verify(ctx context.Context, log *slog.Logger, claims token.Claims, versions VersionChecker) (Identity, error) {
	if claims.Type != token.TypeAccess {
		return Identity{}, ErrNotAccessToken
	}

	if versions != nil {
		current, err := versions.AuthVersion(ctx, claims.UserID)
		switch {
		case err != nil:
			log.Warn("auth version store unavailable, failing open",
				slog.Int("user_id", claims.UserID), logger.Err(err))
		case claims.Version != current:
			return Identity{}, ErrRevoked
		}
	}

	return Identity{UserID: claims.UserID, Role: models.ParseRole(claims.Role)}, nil
}
