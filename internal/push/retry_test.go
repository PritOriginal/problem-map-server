package push_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/push"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRetryAfter(t *testing.T) {
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	past := time.Now().Add(-30 * time.Second).UTC().Format(http.TimeFormat)

	tests := []struct {
		name   string
		header string
		min    time.Duration
		max    time.Duration
	}{
		{name: "Empty", header: "", min: 0, max: 0},
		{name: "Seconds", header: "7", min: 7 * time.Second, max: 7 * time.Second},
		{name: "NegativeSeconds", header: "-3", min: 0, max: 0},
		{name: "HTTPDateFuture", header: future, min: 25 * time.Second, max: 30 * time.Second},
		{name: "HTTPDatePast", header: past, min: 0, max: 0},
		{name: "Garbage", header: "soon", min: 0, max: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := push.ParseRetryAfter(tt.header)
			assert.GreaterOrEqual(t, got, tt.min)
			assert.LessOrEqual(t, got, tt.max)
		})
	}
}

func TestDelay(t *testing.T) {
	const base = 100 * time.Millisecond

	tests := []struct {
		name       string
		attempt    int
		retryAfter time.Duration
		min        time.Duration
		max        time.Duration
	}{
		{name: "First", attempt: 0, min: 100 * time.Millisecond, max: 150 * time.Millisecond},
		{name: "Doubled", attempt: 2, min: 400 * time.Millisecond, max: 600 * time.Millisecond},
		{name: "RetryAfterWins", attempt: 0, retryAfter: 3 * time.Second, min: 3 * time.Second, max: 3 * time.Second},
		{name: "Capped", attempt: 40, min: push.MaxBackoff, max: push.MaxBackoff},
		{name: "RetryAfterCapped", attempt: 0, retryAfter: time.Minute, min: push.MaxBackoff, max: push.MaxBackoff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := push.Delay(base, tt.attempt, tt.retryAfter)
			assert.GreaterOrEqual(t, got, tt.min)
			assert.LessOrEqual(t, got, tt.max)
		})
	}
}

// step is one scripted outcome of an Attempt.
type step struct {
	retry bool
	err   error
}

func TestRetry(t *testing.T) {
	errTransient := errors.New("transient")
	errPermanent := errors.New("permanent")

	tests := []struct {
		name       string
		maxRetries int
		script     []step
		wantErr    error
		wantCalls  int
	}{
		{name: "FirstOk", maxRetries: 3, script: []step{{false, nil}}, wantCalls: 1},
		{name: "TransientThenOk", maxRetries: 3, script: []step{{true, errTransient}, {true, errTransient}, {false, nil}}, wantCalls: 3},
		{name: "Exhausted", maxRetries: 2, script: []step{{true, errTransient}}, wantErr: errTransient, wantCalls: 3},
		{name: "NoRetries", maxRetries: 0, script: []step{{true, errTransient}}, wantErr: errTransient, wantCalls: 1},
		{name: "PermanentNotRetried", maxRetries: 3, script: []step{{false, errPermanent}}, wantErr: errPermanent, wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			err := push.Retry(context.Background(), slogdiscard.NewDiscardLogger(), tt.maxRetries, time.Millisecond,
				func(context.Context) (time.Duration, bool, error) {
					s := tt.script[min(calls, len(tt.script)-1)]
					calls++
					return 0, s.retry, s.err
				})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCalls, calls)
		})
	}

	t.Run("ContextCancelledWhileWaiting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := push.Retry(ctx, slogdiscard.NewDiscardLogger(), 3, time.Minute,
			func(context.Context) (time.Duration, bool, error) {
				cancel()
				return 0, true, errTransient
			})
		require.ErrorIs(t, err, context.Canceled)
	})
}
