// Package idempotency implements the Idempotency-Key protocol for mutating
// endpoints: the response of the first request with a key is stored in
// Redis and replayed for every repeat of the same request within the TTL.
//
// Known limits:
//   - The key is scoped to the authenticated user, so the routes must sit
//     behind the JWT middleware.
//   - The in-progress lock is released when the handler answers 5xx or
//     panics. If the client connection drops after the lock was taken but
//     before the response was stored (the handler may still complete the
//     write), a retry within LockTTL gets 409 and the client must retry
//     after it; a retry after a stored response gets the replay.
//   - The fingerprint of a multipart request covers the form fields only,
//     not the uploaded files: a repeat with the same key and fields but
//     other files is treated as the same request and replayed.
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// Header carries the client-chosen key (a UUID).
	Header = "Idempotency-Key"
	// ReplayedHeader marks a response served from the store.
	ReplayedHeader = "Idempotent-Replayed"

	// MaxKeyLen is the longest accepted key.
	MaxKeyLen = 64
	// MaxStoredBody caps the size of a response kept for replay (256 KiB);
	// a bigger response is served but not stored.
	MaxStoredBody = 256 << 10

	// MsgInProgress is returned (409) while the first request with the key
	// is still being handled.
	MsgInProgress = "request in progress"
	// MsgPayloadMismatch is returned (422) when the key is reused with a
	// different request body.
	MsgPayloadMismatch = "idempotency key reused with different payload"
	// MsgInvalidKey is returned (400) for a malformed key.
	MsgInvalidKey = "invalid Idempotency-Key: expected a UUID"

	// multipartMemory is how much of a multipart body is kept in memory
	// while hashing its fields; the rest spills to temporary files, exactly
	// as Gin's own binding does (the parsed form is reused by the handler).
	multipartMemory = 32 << 20
)

// storedHeaders are the response headers replayed with the body.
var storedHeaders = []string{"Content-Type", "Location"}

// Store is the key-value backend (Redis). GetBytes must report a missing
// key as repository.ErrNotFound; any other error makes the middleware fail
// open.
type Store interface {
	GetBytes(ctx context.Context, key string) ([]byte, error)
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) (bool, error)
	Set(ctx context.Context, key string, value any, expiration time.Duration) error
	Del(ctx context.Context, key string) error
}

// Config tunes the middleware; see config.IdempotencyConfig.
type Config struct {
	// TTL is how long a stored response lives.
	TTL time.Duration
	// LockTTL bounds the in-progress lock of the first request.
	LockTTL time.Duration
}

// record is what the store holds under a key: first the lock (InProgress),
// then the response.
type record struct {
	// Fingerprint identifies the request payload so that a key reused with
	// a different body is rejected.
	Fingerprint string `json:"fingerprint"`
	InProgress  bool   `json:"in_progress,omitempty"`

	Status  int               `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

// Key builds the store key of a user's idempotency key.
func Key(userID int, key string) string {
	return fmt.Sprintf("idem:%d:%s", userID, key)
}

// New returns the middleware. It must run after the JWT middleware (the key
// is scoped to the user) and after MaxBodySize. Requests without the header
// pass through untouched; when the store is nil or unavailable the request
// is handled as usual and a warning is logged (fail open).
func New(log *slog.Logger, store Store, cfg Config) gin.HandlerFunc {
	if cfg.TTL <= 0 {
		cfg.TTL = 24 * time.Hour
	}
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = 30 * time.Second
	}

	return func(c *gin.Context) {
		key := c.GetHeader(Header)
		if key == "" {
			c.Next()
			return
		}
		if len(key) > MaxKeyLen || uuid.Validate(key) != nil {
			responses.BadRequest(c, MsgInvalidKey)
			c.Abort()
			return
		}
		if store == nil {
			c.Next()
			return
		}

		userID, err := middleware.UserIDFromClaims(c)
		if err != nil {
			responses.Unauthorized(c, "invalid token")
			c.Abort()
			return
		}

		fingerprint, err := requestFingerprint(c)
		if err != nil {
			log.Debug("failed read request for idempotency", logger.Err(err))
			responses.BadRequest(c, "invalid request")
			c.Abort()
			return
		}

		ctx := c.Request.Context()
		storeKey := Key(userID, key)

		// Fast path: a finished or in-flight request with this key.
		if rec, found, ok := load(ctx, log, store, storeKey); !ok {
			c.Next()
			return
		} else if found {
			serveStored(c, rec, fingerprint)
			return
		}

		acquired, err := store.SetNX(ctx, storeKey, mustJSON(record{Fingerprint: fingerprint, InProgress: true}), cfg.LockTTL)
		if err != nil {
			log.Warn("idempotency store unavailable, failing open", logger.Err(err))
			c.Next()
			return
		}
		if !acquired {
			// Lost the race with a concurrent request holding the same key.
			rec, found, ok := load(ctx, log, store, storeKey)
			if !ok {
				c.Next()
				return
			}
			if !found {
				// The lock vanished in between (handler failed and released
				// it); the client may simply retry.
				responses.Fail(c, http.StatusConflict, MsgInProgress)
				c.Abort()
				return
			}
			serveStored(c, rec, fingerprint)
			return
		}

		w := &bufferedWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = w

		release := func() {
			// Release the key so that the client can retry.
			if err := store.Del(ctx, storeKey); err != nil {
				log.Warn("failed to release idempotency key", logger.Err(err))
			}
		}
		func() {
			// A panicking handler (turned into 500 by gin.Recovery further
			// up the chain) must not leave the lock in place for LockTTL.
			defer func() {
				if r := recover(); r != nil {
					release()
					panic(r)
				}
			}()
			c.Next()
		}()

		status := w.Status()
		storable := (status >= 200 && status < 500) && w.body.Len() <= MaxStoredBody
		if !storable {
			release()
			return
		}

		rec := record{
			Fingerprint: fingerprint,
			Status:      status,
			Headers:     make(map[string]string, len(storedHeaders)),
			Body:        w.body.Bytes(),
		}
		for _, h := range storedHeaders {
			if v := c.Writer.Header().Get(h); v != "" {
				rec.Headers[h] = v
			}
		}
		if err := store.Set(ctx, storeKey, mustJSON(rec), cfg.TTL); err != nil {
			log.Warn("failed to store idempotent response", logger.Err(err))
		}
	}
}

// load reads the record under key. found is false on a miss; ok is false
// when the store failed (already logged) and the caller should fail open.
func load(ctx context.Context, log *slog.Logger, store Store, key string) (rec record, found, ok bool) {
	data, err := store.GetBytes(ctx, key)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return record{}, false, true
	case err != nil:
		log.Warn("idempotency store unavailable, failing open", logger.Err(err))
		return record{}, false, false
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		log.Warn("corrupted idempotency record, failing open", slog.String("key", key), logger.Err(err))
		return record{}, false, false
	}
	return rec, true, true
}

// serveStored answers a repeated request from the record: 422 for another
// payload, 409 while the first request is in flight, otherwise the replay.
func serveStored(c *gin.Context, rec record, fingerprint string) {
	defer c.Abort()

	if rec.Fingerprint != fingerprint {
		responses.Fail(c, http.StatusUnprocessableEntity, MsgPayloadMismatch)
		return
	}
	if rec.InProgress {
		responses.Fail(c, http.StatusConflict, MsgInProgress)
		return
	}

	for h, v := range rec.Headers {
		c.Header(h, v)
	}
	c.Header(ReplayedHeader, "true")
	contentType := rec.Headers["Content-Type"]
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(rec.Status, contentType, rec.Body)
}

// requestFingerprint hashes what identifies the payload: method, path and
// either the raw body (JSON, urlencoded forms) or, for multipart uploads,
// the sorted form fields without the files. The body is left readable for
// the handler.
func requestFingerprint(c *gin.Context) (string, error) {
	h := sha256.New()
	h.Write([]byte(c.Request.Method))
	h.Write([]byte{0})
	h.Write([]byte(c.Request.URL.RequestURI()))
	h.Write([]byte{0})

	contentType := c.ContentType()
	switch {
	case strings.HasPrefix(contentType, "multipart/"):
		if err := c.Request.ParseMultipartForm(multipartMemory); err != nil {
			return "", err
		}
		form := c.Request.MultipartForm
		if form == nil {
			break
		}
		names := make([]string, 0, len(form.Value))
		for name := range form.Value {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			values := append([]string(nil), form.Value[name]...)
			sort.Strings(values)
			for _, v := range values {
				h.Write([]byte(name))
				h.Write([]byte{'='})
				h.Write([]byte(v))
				h.Write([]byte{0})
			}
		}
	case c.Request.Body != nil:
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return "", err
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		h.Write(body)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // record only holds plain fields
	}
	return b
}

// bufferedWriter copies the response body so it can be stored after the
// handler ran; the bytes still go to the client as they are written.
type bufferedWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bufferedWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *bufferedWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
