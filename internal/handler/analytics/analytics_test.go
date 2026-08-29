package analyticsrest_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	analyticsrest "github.com/PritOriginal/problem-map-server/internal/handler/analytics"
	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type AnalyticsSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *analyticsrest.MockAnalytics
}

func (suite *AnalyticsSuite) SetupTest() {
	suite.uc = analyticsrest.NewMockAnalytics(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()

	analyticsrest.Register(suite.r, slogdiscard.NewDiscardLogger(), suite.uc)
}

func TestAnalytics(t *testing.T) {
	suite.Run(t, new(AnalyticsSuite))
}

var errInvalid = fmt.Errorf("op: %w: bad", usecase.ErrInvalidArgument)

func (suite *AnalyticsSuite) TestGetKPI() {
	tests := []struct {
		name       string
		query      string
		wantCall   bool
		errGetKPI  error
		statusCode int
	}{
		{name: "Ok200", query: "", wantCall: true, statusCode: http.StatusOK},
		{name: "Ok200Filters", query: "?boundary_id=1&mark_type_id=2&from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z", wantCall: true, statusCode: http.StatusOK},
		{name: "Err400NegativeBoundary", query: "?boundary_id=-1", statusCode: http.StatusBadRequest},
		{name: "Err400BoundaryNotANumber", query: "?boundary_id=a", statusCode: http.StatusBadRequest},
		{name: "Err400BadFrom", query: "?from=yesterday", statusCode: http.StatusBadRequest},
		{name: "Err400BadTo", query: "?to=2026-01-01", statusCode: http.StatusBadRequest},
		{name: "Err400Usecase", query: "?from=2026-02-01T00:00:00Z&to=2026-01-01T00:00:00Z", wantCall: true, errGetKPI: errInvalid, statusCode: http.StatusBadRequest},
		{name: "Err500", query: "", wantCall: true, errGetKPI: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCall {
				suite.uc.On("GetKPI", mock.Anything, mock.Anything).Once().
					Return(models.KPI{ByStatus: map[int]int{}}, tt.errGetKPI)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/analytics/kpi"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *AnalyticsSuite) TestGetTimeseries() {
	tests := []struct {
		name             string
		query            string
		wantCall         bool
		wantStep         models.TimeseriesStep
		errGetTimeseries error
		statusCode       int
	}{
		{name: "Ok200Defaults", query: "", wantCall: true, statusCode: http.StatusOK},
		{name: "Ok200Week", query: "?step=week&boundary_id=1", wantCall: true, wantStep: models.StepWeek, statusCode: http.StatusOK},
		{name: "Ok200Month", query: "?step=month&from=2026-01-01T00:00:00Z", wantCall: true, wantStep: models.StepMonth, statusCode: http.StatusOK},
		{name: "Err400Step", query: "?step=hour", statusCode: http.StatusBadRequest},
		{name: "Err400BadFrom", query: "?from=x", statusCode: http.StatusBadRequest},
		{name: "Err400MarkType", query: "?mark_type_id=0.5", statusCode: http.StatusBadRequest},
		{name: "Err400Usecase", query: "?step=day", wantCall: true, wantStep: models.StepDay, errGetTimeseries: errInvalid, statusCode: http.StatusBadRequest},
		{name: "Err500", query: "?step=day", wantCall: true, wantStep: models.StepDay, errGetTimeseries: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCall {
				suite.uc.On("GetTimeseries", mock.Anything, mock.MatchedBy(func(f models.TimeseriesFilters) bool {
					return f.Step == tt.wantStep
				})).Once().Return([]models.TimeseriesPoint{}, tt.errGetTimeseries)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/analytics/timeseries"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *AnalyticsSuite) TestGetTopTypes() {
	tests := []struct {
		name           string
		query          string
		wantCall       bool
		errGetTopTypes error
		statusCode     int
	}{
		{name: "Ok200", query: "", wantCall: true, statusCode: http.StatusOK},
		{name: "Ok200Limit", query: "?limit=3&boundary_id=2&to=2026-01-01T00:00:00Z", wantCall: true, statusCode: http.StatusOK},
		{name: "Ok200LimitZeroMeansDefault", query: "?limit=0", wantCall: true, statusCode: http.StatusOK},
		{name: "Err400LimitTooBig", query: "?limit=101", statusCode: http.StatusBadRequest},
		{name: "Err400LimitNegative", query: "?limit=-1", statusCode: http.StatusBadRequest},
		{name: "Err400BadTo", query: "?to=x", statusCode: http.StatusBadRequest},
		{name: "Err500", query: "", wantCall: true, errGetTopTypes: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCall {
				suite.uc.On("GetTopTypes", mock.Anything, mock.Anything).Once().
					Return([]models.TopType{}, tt.errGetTopTypes)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/analytics/top-types"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}
