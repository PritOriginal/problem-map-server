// Package apikey authenticates requests of the open-data API with an API
// key: header X-Api-Key or "Authorization: ApiKey <key>". The key is
// optional on the public read routes; a request carrying one is rate
// limited per key instead of per IP and may only read.
package apikey

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware/ratelimit"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
)

// Header carries the API key; the Authorization scheme is an alternative.
const (
	Header     = "X-Api-Key"
	AuthScheme = "ApiKey"
)

// Window is the fixed window of the per-key limit (rate_limit_per_min).
const Window = time.Minute

// Authenticator verifies a raw key (usecase.APIKeys).
type Authenticator interface {
	Authenticate(ctx context.Context, raw string) (models.APIKey, error)
}

// Recorder counts the requests made with API keys (metrics).
type Recorder interface {
	APIKeyRequest(keyPrefix string, status int)
}

// Params configures Optional. Counter and Recorder may be nil (no per-key
// limit / no metrics).
type Params struct {
	Auth     Authenticator
	Counter  ratelimit.Counter
	Recorder Recorder
}

// Optional returns the middleware. Requests without a key pass through
// untouched. With a key:
//   - an unknown, revoked or expired key is answered 401;
//   - a non-read method (anything but GET/HEAD/OPTIONS) is answered 403,
//     keys are read-only whatever their scopes;
//   - the request is counted under "apikey:<id>" against the key's own
//     limit, X-RateLimit-* headers are set and 429 is answered over it;
//   - the identity is put into the request context
//     (models.APIKeyFromContext), api_key_id is added to the access log and
//     the request is recorded in the metrics by key prefix and status.
func Optional(log *slog.Logger, p Params) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := FromRequest(c)
		if !ok {
			c.Next()
			return
		}

		key, err := p.Auth.Authenticate(c.Request.Context(), raw)
		if err != nil {
			if usecase.Kind(err) == usecase.KindUnauthorized {
				responses.Unauthorized(c, unauthorizedMessage(err))
			} else {
				log.Error("failed to authenticate api key", logger.Err(err))
				responses.Internal(c, responses.MsgInternal)
			}
			c.Abort()
			return
		}

		identity := key.Identity()
		c.Request = c.Request.WithContext(models.ContextWithAPIKey(c.Request.Context(), identity))
		sloggin.AddCustomAttributes(c, slog.Int("api_key_id", key.ID))
		if p.Recorder != nil {
			defer func() { p.Recorder.APIKeyRequest(key.Prefix, c.Writer.Status()) }()
		}

		if !readMethod(c.Request.Method) {
			responses.Forbidden(c, "api keys are read-only")
			c.Abort()
			return
		}

		if !ratelimit.Allow(c, log, p.Counter, "apikey:"+strconv.Itoa(key.ID), ratelimit.Config{
			Requests: key.RateLimitPerMin,
			Window:   Window,
			Headers:  true,
		}) {
			return
		}

		c.Next()
	}
}

// FromRequest extracts the raw key from X-Api-Key or from
// "Authorization: ApiKey <key>".
func FromRequest(c *gin.Context) (string, bool) {
	if v := strings.TrimSpace(c.GetHeader(Header)); v != "" {
		return v, true
	}
	scheme, value, found := strings.Cut(c.GetHeader("Authorization"), " ")
	if found && strings.EqualFold(scheme, AuthScheme) {
		if value = strings.TrimSpace(value); value != "" {
			return value, true
		}
	}
	return "", false
}

// SkipWithKey wraps a middleware so it is bypassed for requests that were
// authenticated with an API key (which carry their own limit).
func SkipWithKey(h gin.HandlerFunc) gin.HandlerFunc {
	if h == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if _, ok := models.APIKeyFromContext(c.Request.Context()); ok {
			c.Next()
			return
		}
		h(c)
	}
}

func readMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func unauthorizedMessage(err error) string {
	switch {
	case errors.Is(err, usecase.ErrAPIKeyRevoked):
		return "api key revoked"
	case errors.Is(err, usecase.ErrAPIKeyExpired):
		return "api key expired"
	default:
		return "invalid api key"
	}
}
