package push

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

const (
	// DefaultBackoff is the base delay between retries of a provider request.
	DefaultBackoff = 500 * time.Millisecond
	// MaxBackoff caps the delay between retries, Retry-After included.
	MaxBackoff = 10 * time.Second
)

// Attempt performs one provider request. retry reports whether err is
// transient (5xx, 429, transport failure) and retryAfter carries the
// server's Retry-After, if any.
type Attempt func(ctx context.Context) (retryAfter time.Duration, retry bool, err error)

// Retry runs attempt until it succeeds, fails with a permanent error, or
// maxRetries retries are used up; between retries it sleeps Delay. A ctx
// done while waiting ends the loop with ctx.Err().
func Retry(ctx context.Context, log *slog.Logger, maxRetries int, backoff time.Duration, attempt Attempt) error {
	for n := 0; ; n++ {
		retryAfter, retry, err := attempt(ctx)
		if err == nil {
			return nil
		}
		if !retry || n >= maxRetries {
			return err
		}

		delay := Delay(backoff, n, retryAfter)
		log.Debug("push request failed, retrying",
			slog.Int("attempt", n+1), slog.Duration("delay", delay), slogger.Err(err))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Delay returns the backoff before retry number attempt+1: base doubled on
// every attempt with up to 50% random jitter (so retries of concurrent
// sends do not align), or the server's Retry-After when it is larger;
// capped by MaxBackoff.
func Delay(base time.Duration, attempt int, retryAfter time.Duration) time.Duration {
	d := base << uint(attempt)
	if d <= 0 || d > MaxBackoff {
		d = MaxBackoff
	}
	d += time.Duration(rand.Int64N(int64(d)/2 + 1)) //nolint:gosec // jitter, not security
	return min(max(d, retryAfter), MaxBackoff)
}

// ParseRetryAfter reads a Retry-After header, either delay-seconds or an
// HTTP-date (RFC 9110); garbage and moments in the past give 0.
func ParseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil {
		return max(time.Duration(secs)*time.Second, 0)
	}
	if at, err := http.ParseTime(header); err == nil {
		return max(time.Until(at), 0)
	}
	return 0
}
