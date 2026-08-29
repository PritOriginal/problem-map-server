package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

// VersionChecker returns the current auth version of a user. It is
// implemented by the Redis repository (usecase.AuthVersionStore).
type VersionChecker interface {
	AuthVersion(ctx context.Context, userID int) (int64, error)
}

// DefaultVersionCacheTTL bounds how long a bumped auth version may go
// unnoticed by the middleware.
const DefaultVersionCacheTTL = 5 * time.Second

// JWTParams configures NewJWT.
type JWTParams struct {
	// Key is the HMAC key of the access tokens.
	Key string
	// Versions is compared with the "ver" claim of every token; nil disables
	// the check. Lookup errors fail open (logged at warn level).
	Versions VersionChecker
	// VersionCacheTTL is the in-memory cache TTL of the versions; zero means
	// DefaultVersionCacheTTL.
	VersionCacheTTL time.Duration
}

// ctxKeyRejected marks a request the Authorizer rejected, so that the
// Unauthorized hook answers 401 (gin-jwt would answer 403 by itself).
const ctxKeyRejected = "middleware.jwt.rejected"

// NewJWT builds and initialises the gin-jwt middleware used by every
// protected route. On top of the signature and expiry checks done by gin-jwt
// it rejects with 401:
//   - tokens that are not access tokens ("typ" != "access"), so a refresh
//     token can never be used as a bearer token;
//   - tokens whose "ver" claim is behind the version reported by Versions
//     (the user changed the password, was assigned another role or logged
//     out everywhere).
//
// When Versions is unavailable the version check is skipped (fail open)
// and a warning is logged, in line with the cache and the rate limiter.
func NewJWT(log *slog.Logger, p JWTParams) (*jwt.GinJWTMiddleware, error) {
	var versions *versionCache
	if p.Versions != nil {
		ttl := p.VersionCacheTTL
		if ttl <= 0 {
			ttl = DefaultVersionCacheTTL
		}
		versions = newVersionCache(p.Versions, ttl)
	}

	mw, err := jwt.New(&jwt.GinJWTMiddleware{
		Key: []byte(p.Key),
		Authorizer: func(c *gin.Context, _ any) bool {
			if reason := checkToken(c, log, versions); reason != "" {
				c.Set(ctxKeyRejected, reason)
				return false
			}
			return true
		},
		Unauthorized: func(c *gin.Context, code int, message string) {
			if reason, ok := c.Get(ctxKeyRejected); ok {
				code, message = http.StatusUnauthorized, reason.(string)
			}
			c.JSON(code, gin.H{"code": code, "message": message})
		},
	})
	if err != nil {
		return nil, err
	}
	if err := mw.MiddlewareInit(); err != nil {
		return nil, err
	}

	return mw, nil
}

// checkToken inspects the verified claims and returns a rejection reason,
// or "" when the token is acceptable.
func checkToken(c *gin.Context, log *slog.Logger, versions *versionCache) string {
	claims, err := token.ParseClaims(jwt.ExtractClaims(c))
	if err != nil {
		return "invalid token"
	}
	if claims.Type != token.TypeAccess {
		return "not an access token"
	}
	if versions == nil {
		return ""
	}

	current, err := versions.get(c.Request.Context(), claims.UserID)
	if err != nil {
		log.Warn("auth version store unavailable, failing open",
			slog.Int("user_id", claims.UserID), logger.Err(err))
		return ""
	}
	if claims.Version != current {
		return "token revoked"
	}

	return ""
}

// versionCache memoises VersionChecker answers for a short time so that
// every request does not hit Redis. Errors are not cached.
type versionCache struct {
	src VersionChecker
	ttl time.Duration

	mu      sync.Mutex
	entries map[int]versionEntry
}

type versionEntry struct {
	version int64
	expires time.Time
}

func newVersionCache(src VersionChecker, ttl time.Duration) *versionCache {
	return &versionCache{src: src, ttl: ttl, entries: make(map[int]versionEntry)}
}

func (vc *versionCache) get(ctx context.Context, userID int) (int64, error) {
	now := time.Now()

	vc.mu.Lock()
	e, ok := vc.entries[userID]
	vc.mu.Unlock()
	if ok && now.Before(e.expires) {
		return e.version, nil
	}

	v, err := vc.src.AuthVersion(ctx, userID)
	if err != nil {
		return 0, err
	}

	vc.mu.Lock()
	// Drop stale entries opportunistically so the map does not grow with
	// every user that ever authenticated.
	for id, entry := range vc.entries {
		if now.After(entry.expires) {
			delete(vc.entries, id)
		}
	}
	vc.entries[userID] = versionEntry{version: v, expires: now.Add(vc.ttl)}
	vc.mu.Unlock()

	return v, nil
}
