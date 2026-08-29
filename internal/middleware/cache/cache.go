// Package cache caches successful responses of read-only endpoints in Redis
// and serves them with a validator (ETag) so that clients holding a copy
// get 304 Not Modified instead of the body.
package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/gin-gonic/gin"
)

type Cacher interface {
	Get(ctx context.Context, key string, v any) error
	GetBytes(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
}

// DefaultMaxAge is the Cache-Control max-age sent with cached responses
// unless WithMaxAge says otherwise.
const DefaultMaxAge = 60 * time.Second

// Option tunes the middleware.
type Option func(*options)

type options struct {
	maxAge time.Duration
}

// WithMaxAge sets the Cache-Control max-age of the responses.
func WithMaxAge(d time.Duration) Option {
	return func(o *options) { o.maxAge = d }
}

// entry is what the cache holds for a key.
type entry struct {
	ETag        string `json:"etag"`
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

// New returns a middleware that buffers a successful (2xx) response, tags it
// with a strong ETag (sha256 of the body), stores it in the cacher for ttl
// and answers a matching If-None-Match with 304. A nil or failing cacher
// only disables the storage: the ETag handling still works on every
// response.
//
// The cache key includes the language resolved by the lang middleware
// (models.LangFromContext) because the cached payload is localised; the
// response is marked Vary: Accept-Language for the same reason.
func New(cacher Cacher, ttl time.Duration, opts ...Option) gin.HandlerFunc {
	o := options{maxAge: DefaultMaxAge}
	for _, opt := range opts {
		opt(&o)
	}
	cacheControl := "public, max-age=" + strconv.Itoa(int(o.maxAge.Seconds()))

	return func(c *gin.Context) {
		if !slices.Contains(c.Writer.Header().Values("Vary"), "Accept-Language") {
			c.Writer.Header().Add("Vary", "Accept-Language")
		}

		cacheKey := Key(c.Request.Method, c.Request.URL.String(), models.LangFromContext(c.Request.Context()))

		if cacher != nil {
			if e, ok := lookup(c.Request.Context(), cacher, cacheKey); ok {
				serve(c, e, cacheControl)
				c.Abort()
				return
			}
		}

		w := &bufferedWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = w

		c.Next()

		c.Writer = w.ResponseWriter
		status := w.status
		if status < 200 || status >= 300 {
			w.flush()
			return
		}

		e := entry{
			ETag:        ETag(w.body.Bytes()),
			ContentType: w.Header().Get("Content-Type"),
			Body:        w.body.Bytes(),
		}
		if cacher != nil {
			_ = cacher.Set(c.Request.Context(), cacheKey, mustJSON(e), ttl)
		}
		c.Status(status)
		serve(c, e, cacheControl)
	}
}

// lookup reads and decodes the entry under key; a miss, a backend error or
// a value in another format all count as "not cached".
func lookup(ctx context.Context, cacher Cacher, key string) (entry, bool) {
	data, err := cacher.GetBytes(ctx, key)
	if err != nil {
		return entry{}, false
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil || e.ETag == "" {
		return entry{}, false
	}
	return e, true
}

// serve writes the entry: validator headers first, then either 304 (the
// client's copy is current) or the body with the status already set on the
// context (200 for a cache hit).
func serve(c *gin.Context, e entry, cacheControl string) {
	c.Header("ETag", e.ETag)
	c.Header("Cache-Control", cacheControl)

	if Matches(c.GetHeader("If-None-Match"), e.ETag) {
		c.Status(http.StatusNotModified)
		c.Writer.WriteHeaderNow()
		return
	}

	status := c.Writer.Status()
	if status == 0 {
		status = http.StatusOK
	}
	contentType := e.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(status, contentType, e.Body)
}

// Key builds the cache key of a request: method, full URL and language.
func Key(method, url string, lang models.Lang) string {
	return fmt.Sprintf("http:%s:%s:%s", method, url, lang)
}

// ETag returns the strong validator of a body: the quoted hex sha256.
func ETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// Matches reports whether an If-None-Match header value matches the ETag:
// "*" or any of its comma-separated entries (weak prefixes ignored).
func Matches(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	if strings.TrimSpace(ifNoneMatch) == "*" {
		return true
	}
	for _, candidate := range strings.Split(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // entry only holds plain fields
	}
	return b
}

// bufferedWriter holds the response back until the handler returned, so
// that the ETag can be computed from the body and put in the headers.
type bufferedWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (w *bufferedWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

func (w *bufferedWriter) WriteString(s string) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.WriteString(s)
}

// WriteHeader records the status; like Gin's writer it may be changed
// until the first byte of the body is written.
func (w *bufferedWriter) WriteHeader(code int) {
	if code > 0 && w.body.Len() == 0 {
		w.status = code
	}
}

// WriteHeaderNow is deferred to flush.
func (w *bufferedWriter) WriteHeaderNow() {}

func (w *bufferedWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *bufferedWriter) Size() int { return w.body.Len() }

func (w *bufferedWriter) Written() bool { return w.status != 0 }

// flush sends the buffered response to the client unchanged.
func (w *bufferedWriter) flush() {
	w.ResponseWriter.WriteHeader(w.Status())
	if w.body.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.body.Bytes())
	} else {
		w.ResponseWriter.WriteHeaderNow()
	}
}
