package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type HealthSuite struct {
	suite.Suite
}

func TestHealth(t *testing.T) {
	suite.Run(t, new(HealthSuite))
}

func (suite *HealthSuite) SetupSuite() {
	gin.SetMode(gin.TestMode)
}

func (suite *HealthSuite) newRouter(checker Checker) *gin.Engine {
	r := gin.New()
	Register(r, slogdiscard.NewDiscardLogger(), checker)
	return r
}

func (suite *HealthSuite) TestHealthz() {
	r := suite.newRouter(NewMockChecker(suite.T()))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, PathLive, nil))

	suite.Equal(http.StatusOK, w.Code)
	suite.JSONEq(`{"success":true,"payload":{"status":"ok"}}`, w.Body.String())
}

func (suite *HealthSuite) TestReadyz() {
	tests := []struct {
		name       string
		report     usecase.HealthReport
		err        error
		statusCode int
	}{
		{
			name:       "Ok200",
			report:     usecase.HealthReport{"postgres": "ok", "redis": "ok"},
			statusCode: http.StatusOK,
		},
		{
			name:       "Err503",
			report:     usecase.HealthReport{"postgres": "error", "redis": "ok"},
			err:        usecase.ErrUnavailable,
			statusCode: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			checker := NewMockChecker(suite.T())
			checker.EXPECT().Check(mock.Anything).Return(tt.report, tt.err)
			r := suite.newRouter(checker)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, PathReady, nil))

			suite.Equal(tt.statusCode, w.Code)
			var got responses.Response[usecase.HealthReport]
			suite.NoError(json.Unmarshal(w.Body.Bytes(), &got))
			suite.Equal(tt.err == nil, got.Success)
			suite.Equal(tt.report, got.Payload)
		})
	}
}
