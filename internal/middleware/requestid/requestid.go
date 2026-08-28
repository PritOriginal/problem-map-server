// Package requestid provides a gin middleware that generates or propagates
// an X-Request-ID header and stores it in the request context and logger.
package requestid

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Header is the HTTP header used to carry the request identifier.
const Header = "X-Request-ID"

// ContextKey is the gin context key under which the request ID is stored.
const ContextKey = "request_id"

type ctxKey struct{}

type loggerKey struct{}

// New returns a middleware that reads X-Request-ID from the incoming request
// (or generates a UUID when absent), echoes it in the response header, stores
// it in the gin context and in the underlying request context, and attaches a
// logger enriched with request_id to the context.
func New(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(Header)
		if id == "" {
			id = uuid.NewString()
		}

		c.Set(ContextKey, id)
		c.Header(Header, id)
		// Keep the incoming header in sync so downstream middleware (e.g.
		// slog-gin) reuses the same identifier instead of generating its own.
		c.Request.Header.Set(Header, id)

		ctx := context.WithValue(c.Request.Context(), ctxKey{}, id)
		ctx = context.WithValue(ctx, loggerKey{}, log.With(slog.String(ContextKey, id)))
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// FromContext returns the request ID stored in ctx, or an empty string.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// FromGin returns the request ID stored in the gin context, or an empty string.
func FromGin(c *gin.Context) string {
	return c.GetString(ContextKey)
}

// Logger returns the request-scoped logger from ctx, falling back to fallback
// when the context carries none.
func Logger(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return fallback
}
