package cache

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Cacher interface {
	Get(ctx context.Context, key string, v any) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
}

func New(cacher Cacher, ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		cacheKey := fmt.Sprintf("http:%s:%s", c.Request.Method, c.Request.URL.String())

		cachedResponse, err := cacher.GetBytes(c.Request.Context(), cacheKey)
		if err == nil {
			c.Data(http.StatusOK, "application/json", cachedResponse)
			c.Abort()
			return
		}

		blw := &bodyLogWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = blw

		c.Next()

		if blw.status >= 200 && blw.status < 300 {
			cacher.Set(c.Request.Context(), cacheKey, blw.body.Bytes(), ttl)
		}
	}
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bodyLogWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
