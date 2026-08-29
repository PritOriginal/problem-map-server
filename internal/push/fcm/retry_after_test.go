package fcm

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
			got := parseRetryAfter(tt.header)
			assert.GreaterOrEqual(t, got, tt.min)
			assert.LessOrEqual(t, got, tt.max)
		})
	}
}

func TestDelay(t *testing.T) {
	s := &Sender{backoff: 100 * time.Millisecond}

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
		{name: "Capped", attempt: 40, min: maxBackoff, max: maxBackoff},
		{name: "RetryAfterCapped", attempt: 0, retryAfter: time.Minute, min: maxBackoff, max: maxBackoff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.delay(tt.attempt, tt.retryAfter)
			assert.GreaterOrEqual(t, got, tt.min)
			assert.LessOrEqual(t, got, tt.max)
		})
	}
}
