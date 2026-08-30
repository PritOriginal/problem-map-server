package middleware

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// rawBodyKey holds the request body as received, before any MaxBodySize
// wrapper, so that a route-level MaxBodySize can replace the router-wide
// limit instead of nesting inside it (nested http.MaxBytesReader keeps the
// smallest limit).
const rawBodyKey = "middleware.rawBody"

// MaxBodySize limits the size of the request body to maxBytes.
// Reading beyond the limit makes the body reader return an error, which
// binding reports as a bad request. Applied again on a route group it
// replaces the router-wide limit for that group (e.g. a larger limit for
// multipart photo uploads).
func MaxBodySize(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}

		raw := c.Request.Body
		if v, ok := c.Get(rawBodyKey); ok {
			raw = v.(io.ReadCloser)
		} else {
			c.Set(rawBodyKey, raw)
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, raw, maxBytes)
		c.Next()
	}
}
