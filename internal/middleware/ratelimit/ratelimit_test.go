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
		errIncr    error
		statusCode int
	}{
		{name: "FirstRequest", path: "/auth/signin", count: 1, statusCode: http.StatusOK},
		{name: "AtLimit", path: "/auth/signin", count: 2, statusCode: http.StatusOK},
		{name: "OverLimit", path: "/auth/signin", count: 3, statusCode: http.StatusTooManyRequests},
		{name: "FailOpen", path: "/auth/signin", errIncr: errors.New("redis down"), statusCode: http.StatusOK},
		{name: "Disabled", path: "/disabled", statusCode: http.StatusOK},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.path != "/disabled" {
				suite.counter.On("Incr", mock.Anything, "ratelimit:/auth/signin:192.0.2.1", time.Minute).Once().
					Return(tt.count, tt.errIncr)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
			if tt.statusCode == http.StatusTooManyRequests {
				suite.Equal("60", w.Header().Get("Retry-After"))
			}
		})
	}
}
