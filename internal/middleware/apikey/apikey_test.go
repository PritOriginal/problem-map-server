package apikey_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware/apikey"
	"github.com/PritOriginal/problem-map-server/internal/middleware/ratelimit"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const rawKey = "pm_live_0123456789abcdef0123456789abcdef"

type APIKeySuite struct {
	suite.Suite
	r        *gin.Engine
	auth     *apikey.MockAuthenticator
	counter  *ratelimit.MockCounter
	recorder *apikey.MockRecorder
}

func (suite *APIKeySuite) SetupTest() {
	suite.auth = apikey.NewMockAuthenticator(suite.T())
	suite.counter = ratelimit.NewMockCounter(suite.T())
	suite.recorder = apikey.NewMockRecorder(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()

	// echo reports the identity the handler sees.
	echo := func(c *gin.Context) {
		id, ok := models.APIKeyFromContext(c.Request.Context())
		c.JSON(http.StatusOK, gin.H{"key_id": id.KeyID, "with_key": ok})
	}
	limited := func(c *gin.Context) { c.Header("X-IP-Limited", "1"); c.Next() }
	// jwt stands in for middleware.OptionalAuth: X-Viewer marks a request
	// whose Bearer token was verified before the key middleware runs.
	jwt := func(c *gin.Context) {
		if c.GetHeader("X-Viewer") != "" {
			c.Request = c.Request.WithContext(models.ContextWithViewer(c.Request.Context(), 42))
		}
		c.Next()
	}

	g := suite.r.Group("", jwt, apikey.Optional(slogdiscard.NewDiscardLogger(), apikey.Params{
		Auth:     suite.auth,
		Counter:  suite.counter,
		Recorder: suite.recorder,
	}))
	g.GET("/marks", echo)
	g.POST("/marks", echo)
	g.GET("/marks/export", apikey.SkipWithKey(limited), echo)
}

func TestAPIKey(t *testing.T) {
	suite.Run(t, new(APIKeySuite))
}

func key() models.APIKey {
	return models.APIKey{ID: 5, OwnerUserID: 7, Prefix: "pm_live_01234567", Scopes: []string{"read"}, RateLimitPerMin: 10, Active: true}
}

func (suite *APIKeySuite) TestOptional() {
	tests := []struct {
		name       string
		method     string
		path       string
		header     map[string]string
		authKey    models.APIKey
		authErr    error
		noAuth     bool
		count      int64
		noCount    bool
		statusCode int
		remaining  string
		retryAfter string
		ipLimited  string
	}{
		{
			name: "NoKeyPassesThrough", method: http.MethodGet, path: "/marks/export",
			noAuth: true, noCount: true, statusCode: http.StatusOK, ipLimited: "1",
		},
		{
			name: "ValidKeyHeader", method: http.MethodGet, path: "/marks",
			header: map[string]string{apikey.Header: rawKey}, authKey: key(), count: 1,
			statusCode: http.StatusOK, remaining: "9",
		},
		{
			name: "ValidKeyAuthorizationScheme", method: http.MethodGet, path: "/marks",
			header: map[string]string{"Authorization": "ApiKey " + rawKey}, authKey: key(), count: 1,
			statusCode: http.StatusOK, remaining: "9",
		},
		{
			name: "BearerIsNotAKey", method: http.MethodGet, path: "/marks",
			header: map[string]string{"Authorization": "Bearer " + rawKey},
			noAuth: true, noCount: true, statusCode: http.StatusOK,
		},
		{
			name: "KeySkipsIPLimiter", method: http.MethodGet, path: "/marks/export",
			header: map[string]string{apikey.Header: rawKey}, authKey: key(), count: 1,
			statusCode: http.StatusOK, remaining: "9", ipLimited: "",
		},
		{
			name: "Unknown401", method: http.MethodGet, path: "/marks",
			header: map[string]string{apikey.Header: "pm_live_nope"}, authErr: usecase.ErrUnauthorized,
			noCount: true, statusCode: http.StatusUnauthorized,
		},
		{
			name: "Revoked401", method: http.MethodGet, path: "/marks",
			header: map[string]string{apikey.Header: rawKey}, authErr: usecase.ErrAPIKeyRevoked,
			noCount: true, statusCode: http.StatusUnauthorized,
		},
		{
			name: "Expired401", method: http.MethodGet, path: "/marks",
			header: map[string]string{apikey.Header: rawKey}, authErr: usecase.ErrAPIKeyExpired,
			noCount: true, statusCode: http.StatusUnauthorized,
		},
		{
			name: "Internal500", method: http.MethodGet, path: "/marks",
			header: map[string]string{apikey.Header: rawKey}, authErr: errors.New("db down"),
			noCount: true, statusCode: http.StatusInternalServerError,
		},
		{
			name: "OverLimit429", method: http.MethodGet, path: "/marks",
			header: map[string]string{apikey.Header: rawKey}, authKey: key(), count: 11,
			statusCode: http.StatusTooManyRequests, remaining: "0", retryAfter: "30",
		},
		{
			name: "Write403", method: http.MethodPost, path: "/marks",
			header: map[string]string{apikey.Header: rawKey}, authKey: key(),
			noCount: true, statusCode: http.StatusForbidden,
		},
		{
			name: "JWTWinsOverKeyOnWrite", method: http.MethodPost, path: "/marks",
			header: map[string]string{apikey.Header: "pm_live_garbage", "X-Viewer": "1"},
			noAuth: true, noCount: true, statusCode: http.StatusOK,
		},
		{
			name: "JWTWinsOverKeyOnRead", method: http.MethodGet, path: "/marks/export",
			header: map[string]string{apikey.Header: rawKey, "X-Viewer": "1"},
			noAuth: true, noCount: true, statusCode: http.StatusOK, ipLimited: "1",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.noAuth {
				suite.auth.On("Authenticate", mock.Anything, mock.Anything).Once().Return(tt.authKey, tt.authErr)
			}
			if !tt.noCount {
				suite.counter.On("Incr", mock.Anything, "apikey:5", time.Minute).Once().Return(tt.count, 30*time.Second, nil)
			}
			if tt.authErr == nil && !tt.noAuth {
				suite.recorder.On("APIKeyRequest", "pm_live_01234567", tt.statusCode).Once().Return()
			}

			req := httptest.NewRequest(tt.method, tt.path, nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code, w.Body.String())
			suite.Equal(tt.remaining, w.Header().Get("X-RateLimit-Remaining"))
			suite.Equal(tt.retryAfter, w.Header().Get("Retry-After"))
			suite.Equal(tt.ipLimited, w.Header().Get("X-IP-Limited"))
			if tt.statusCode == http.StatusOK && tt.authErr == nil && !tt.noAuth {
				suite.Contains(w.Body.String(), `"key_id":5`)
				suite.Contains(w.Body.String(), `"with_key":true`)
			}
		})
	}
}

func (suite *APIKeySuite) TestFailOpenWithoutCounter() {
	r := gin.New()
	r.GET("/marks", apikey.Optional(slogdiscard.NewDiscardLogger(), apikey.Params{Auth: suite.auth}), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	suite.auth.On("Authenticate", mock.Anything, rawKey).Once().Return(key(), nil)

	req := httptest.NewRequest(http.MethodGet, "/marks", nil)
	req.Header.Set(apikey.Header, rawKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	suite.Equal(http.StatusOK, w.Code)
	suite.Empty(w.Header().Get("X-RateLimit-Limit"))
}
