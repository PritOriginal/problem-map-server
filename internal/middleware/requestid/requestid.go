// Package requestid provides a gin middleware that propagates or generates
// an X-Request-ID header. The identifier is stored on the request so that
// downstream middleware (slog-gin) logs the same value.
package requestid

import (
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Header is the HTTP header used to carry the request identifier.
const Header = "X-Request-ID"

// ContextKey is the gin context key under which the request ID is stored.
const ContextKey = "request_id"

const maxIDLength = 128

// validID bounds what a client may inject into response headers and logs:
// printable ASCII without spaces or control characters.
var validID = regexp.MustCompile(`^[\x21-\x7E]+$`)

// New returns a middleware that reads X-Request-ID from the incoming request
// (generating a UUID when absent or malformed), echoes it in the response
// header and stores it in the gin context.
func New() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(Header)
		if len(id) > maxIDLength || !validID.MatchString(id) {
			id = uuid.NewString()
		}

		c.Set(ContextKey, id)
		c.Header(Header, id)
		// Keep the incoming header in sync so slog-gin reuses this identifier
		// instead of generating its own.
		c.Request.Header.Set(Header, id)

		c.Next()
	}
}
