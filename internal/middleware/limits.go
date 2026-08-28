package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MaxBodySize limits the size of the request body to maxBytes.
// Reading beyond the limit makes the body reader return an error, which
// binding reports as a bad request.
func MaxBodySize(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
