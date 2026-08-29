package cache

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/gin-gonic/gin"
)

type Cacher interface {
	Get(ctx context.Context, key string, v any) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
}

// New returns a middleware caching successful JSON responses. A nil cacher
// disables caching: the middleware just passes the request through.
//
// The cache key includes the language resolved by the lang middleware
// (models.LangFromContext) because the cached payload is localised; the
// response is marked Vary: Accept-Language for the same reason.
func New(cacher Cacher, ttl time.Duration) gin.HandlerFunc {
	if cacher == nil {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		if !slices.Contains(c.Writer.Header().Values("Vary"), "Accept-Language") {
			c.Writer.Header().Add("Vary", "Accept-Language")
		}

		cacheKey := Key(c.Request.Method, c.Request.URL.String(), models.LangFromContext(c.Request.Context()))

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
			_ = cacher.Set(c.Request.Context(), cacheKey, blw.body.Bytes(), ttl)
		}
	}
}

// Key builds the cache key of a request: method, full URL and language.
func Key(method, url string, lang models.Lang) string {
	return fmt.Sprintf("http:%s:%s:%s", method, url, lang)
}

// Prefix is the common prefix of the keys of every cached response of the
// route (any query string, any language); it is what an invalidation
// (e.g. usecase.DictionaryCache.DeleteByPrefix) matches on.
func Prefix(method, path string) string {
	return fmt.Sprintf("http:%s:%s", method, path)
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
