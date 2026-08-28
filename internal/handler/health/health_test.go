package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/handler/health"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
)

type pingerFunc func(ctx context.Context) error

func (f pingerFunc) Ping(ctx context.Context) error { return f(ctx) }

func okPinger() health.Pinger {
	return pingerFunc(func(context.Context) error { return nil })
}

func failPinger() health.Pinger {
	return pingerFunc(func(context.Context) error { return errors.New("down") })
}

type HealthSuite struct {
	suite.Suite
}

func TestHealth(t *testing.T) {
	suite.Run(t, new(HealthSuite))
}

func (suite *HealthSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
}

func (suite *HealthSuite) newRouter(deps health.Dependencies) *gin.Engine {
	r := gin.New()
	health.Register(r, slogdiscard.NewDiscardLogger(), deps)
	return r
}

func (suite *HealthSuite) TestHealthz() {
	r := suite.newRouter(health.Dependencies{"postgres": failPinger()})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	suite.Equal(http.StatusOK, w.Code)
	suite.JSONEq(`{"status":"ok"}`, w.Body.String())
}

func (suite *HealthSuite) TestReadyz() {
	tests := []struct {
		name       string
		deps       health.Dependencies
		statusCode int
		want       map[string]string
	}{
		{
			name:       "Ok200",
			deps:       health.Dependencies{"postgres": okPinger(), "redis": okPinger()},
			statusCode: http.StatusOK,
			want:       map[string]string{"postgres": "ok", "redis": "ok"},
		},
		{
			name:       "Err503Postgres",
			deps:       health.Dependencies{"postgres": failPinger(), "redis": okPinger()},
			statusCode: http.StatusServiceUnavailable,
			want:       map[string]string{"postgres": "error", "redis": "ok"},
		},
		{
			name:       "Err503All",
			deps:       health.Dependencies{"postgres": failPinger(), "redis": failPinger()},
			statusCode: http.StatusServiceUnavailable,
			want:       map[string]string{"postgres": "error", "redis": "error"},
		},
		{
			name:       "Ok200NilDependency",
			deps:       health.Dependencies{"postgres": okPinger(), "redis": nil},
			statusCode: http.StatusOK,
			want:       map[string]string{"postgres": "ok", "redis": "ok"},
		},
		{
			name:       "Ok200NoDependencies",
			deps:       health.Dependencies{},
			statusCode: http.StatusOK,
			want:       map[string]string{},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			r := suite.newRouter(tt.deps)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			suite.Equal(tt.statusCode, w.Code)
			var got map[string]string
			suite.NoError(json.Unmarshal(w.Body.Bytes(), &got))
			suite.Equal(tt.want, got)
		})
	}
}
