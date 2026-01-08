package cache

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"
)

type Cacher interface {
	Get(ctx context.Context, key string, v any) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
}

func New(cacher Cacher, ttl time.Duration) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cacheKey := fmt.Sprintf("http:%s:%s", r.Method, r.URL.String())

			cachedResponse, err := cacher.GetBytes(r.Context(), cacheKey)
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.Write(cachedResponse)
				return
			}

			rw := &responseWriter{
				ResponseWriter: w,
				body:           &bytes.Buffer{},
			}

			next.ServeHTTP(rw, r)

			if rw.Status() >= 200 && rw.Status() < 300 {
				cacher.Set(context.Background(), cacheKey, rw.body.Bytes(), ttl)
			}

			w.Write(rw.body.Bytes())
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.body.Write(b)
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.status = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Status() int {
	return rw.status
}
