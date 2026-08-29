package ratelimit_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware/ratelimit"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/gin-gonic/gin"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type RateLimitSuite struct {
	suite.Suite
	r       *gin.Engine
	counter *ratelimit.MockCounter
}

func (suite *RateLimitSuite) SetupSuite() {
	suite.counter = ratelimit.NewMockCounter(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()

	limited := suite.r.Group("", ratelimit.New(slogdiscard.NewDiscardLogger(), suite.counter, ratelimit.Config{
		Requests: 2,
		Window:   time.Minute,
	}))
	limited.POST("/auth/signin", func(c *gin.Context) { c.Status(http.StatusOK) })

	disabled := suite.r.Group("", ratelimit.New(slogdiscard.NewDiscardLogger(), suite.counter, ratelimit.Config{}))
	disabled.POST("/disabled", func(c *gin.Context) { c.Status(http.StatusOK) })
}

func TestRateLimit(t *testing.T) {
	suite.Run(t, new(RateLimitSuite))
}

func (suite *RateLimitSuite) TestNew() {
	tests := []struct {
		name       string
		path       string
		count      int64
		ttl        time.Duration
		errIncr    error
		statusCode int
		retryAfter string
	}{
		{name: "FirstRequest", path: "/auth/signin", count: 1, ttl: time.Minute, statusCode: http.StatusOK},
		{name: "AtLimit", path: "/auth/signin", count: 2, ttl: 30 * time.Second, statusCode: http.StatusOK},
		{name: "OverLimit", path: "/auth/signin", count: 3, ttl: 42 * time.Second, statusCode: http.StatusTooManyRequests, retryAfter: "42"},
		{name: "OverLimitRoundsUp", path: "/auth/signin", count: 3, ttl: 1500 * time.Millisecond, statusCode: http.StatusTooManyRequests, retryAfter: "2"},
		{name: "OverLimitNoTTLFallsBackToWindow", path: "/auth/signin", count: 3, ttl: 0, statusCode: http.StatusTooManyRequests, retryAfter: "60"},
		{name: "OverLimitTTLAboveWindowClamped", path: "/auth/signin", count: 3, ttl: time.Hour, statusCode: http.StatusTooManyRequests, retryAfter: "60"},
		{name: "FailOpen", path: "/auth/signin", errIncr: errors.New("redis down"), statusCode: http.StatusOK},
		{name: "Disabled", path: "/disabled", statusCode: http.StatusOK},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.path != "/disabled" {
				suite.counter.On("Incr", mock.Anything, "ratelimit:/auth/signin:192.0.2.1", time.Minute).Once().
					Return(tt.count, tt.ttl, tt.errIncr)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
			suite.Equal(tt.retryAfter, w.Header().Get("Retry-After"))
		})
	}
}
