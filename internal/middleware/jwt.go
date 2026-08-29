package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/auth"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
)

// VersionChecker returns the current auth version of a user. It is
// implemented by the Redis repository (usecase.AuthVersionStore) and by
// VersionCache; the mock lives in package auth.
type VersionChecker = auth.VersionChecker

// VersionSource is what VersionCache wraps: the Redis repository, which can
// both read and bump the auth version.
type VersionSource interface {
	VersionChecker
	IncrAuthVersion(ctx context.Context, userID int) (int64, error)
}

// DefaultVersionCacheTTL bounds how long a bumped auth version may go
// unnoticed by the middleware.
const DefaultVersionCacheTTL = 5 * time.Second

// JWTParams configures NewJWT.
type JWTParams struct {
	// Key is the HMAC key of the access tokens.
	Key string
	// Versions is compared with the "ver" claim of every token; nil disables
	// the check. Lookup errors fail open (logged at warn level). Pass a
	// *VersionCache shared with the usecases so that a bump made by this
	// process is visible to the middleware at once; any other checker is
	// wrapped in a private cache.
	Versions VersionChecker
	// VersionCacheTTL is the in-memory cache TTL of the versions; zero means
	// DefaultVersionCacheTTL. Ignored when Versions is already a *VersionCache.
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
	var versions *VersionCache
	switch v := p.Versions.(type) {
	case nil:
	case *VersionCache:
		versions = v
	default:
		versions = NewVersionCache(readOnlyVersions{v}, p.VersionCacheTTL)
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
// or "" when the token is acceptable. The checks themselves live in
// internal/auth and are shared with the gRPC interceptor.
func checkToken(c *gin.Context, log *slog.Logger, versions *VersionCache) string {
	claims, err := token.ParseClaims(jwt.ExtractClaims(c))
	if err != nil {
		return "invalid token"
	}

	var checker auth.VersionChecker
	if versions != nil {
		checker = versions
	}
	if _, err := auth.Verify(c.Request.Context(), log, claims, checker); err != nil {
		return err.Error()
	}

	return ""
}

// VersionCache memoises the auth versions of a VersionSource for a short
// time so that every request does not hit Redis. Errors are not cached.
// IncrAuthVersion is forwarded to the source and drops the cached entry, so
// a "logout everywhere" served by this process takes effect immediately
// here; other instances notice it within the TTL. Share one VersionCache
// between NewJWT and the usecases (it satisfies usecase.AuthVersionStore).
type VersionCache struct {
	src VersionSource
	ttl time.Duration

	mu      sync.Mutex
	entries map[int]versionEntry
}

type versionEntry struct {
	version int64
	expires time.Time
}

// NewVersionCache wraps src; a non-positive ttl means DefaultVersionCacheTTL.
func NewVersionCache(src VersionSource, ttl time.Duration) *VersionCache {
	if ttl <= 0 {
		ttl = DefaultVersionCacheTTL
	}
	return &VersionCache{src: src, ttl: ttl, entries: make(map[int]versionEntry)}
}

// AuthVersion returns the cached version of the user, asking the source
// when the entry is missing or expired.
func (vc *VersionCache) AuthVersion(ctx context.Context, userID int) (int64, error) {
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

// IncrAuthVersion bumps the version at the source and forgets the cached
// value, whether or not the bump succeeded.
func (vc *VersionCache) IncrAuthVersion(ctx context.Context, userID int) (int64, error) {
	vc.Invalidate(userID)
	v, err := vc.src.IncrAuthVersion(ctx, userID)
	vc.Invalidate(userID)
	return v, err
}

// Invalidate drops the cached version of the user.
func (vc *VersionCache) Invalidate(userID int) {
	vc.mu.Lock()
	delete(vc.entries, userID)
	vc.mu.Unlock()
}

// readOnlyVersions adapts a plain VersionChecker to VersionSource for the
// private cache built by NewJWT.
type readOnlyVersions struct{ VersionChecker }

func (readOnlyVersions) IncrAuthVersion(context.Context, int) (int64, error) {
	return 0, errors.New("middleware: auth version store is read-only")
}
