package openrest_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	openrest "github.com/PritOriginal/problem-map-server/internal/handler/open"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/gin-gonic/gin"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var errInternal = errors.New("boom")

type OpenSuite struct {
	suite.Suite
	r      *gin.Engine
	uc     *openrest.MockStats
	cacher *mwcache.MockCacher
}

func (suite *OpenSuite) SetupTest() {
	suite.uc = openrest.NewMockStats(suite.T())
	suite.cacher = mwcache.NewMockCacher(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	openrest.Register(suite.r, slogdiscard.NewDiscardLogger(), suite.uc, suite.cacher)
}

func TestOpen(t *testing.T) {
	suite.Run(t, new(OpenSuite))
}

func stats() models.OpenStats {
	return models.OpenStats{
		MarksTotal:      3,
		ByStatus:        map[int]int{1: 2, 5: 1},
		ByType:          []models.TypeCount{{Code: "garbage", Count: 2}, {Code: "lighting", Count: 1}},
		ResolvedLast30d: 1,
		AvgCloseHours:   null.FloatFrom(36),
	}
}

func (suite *OpenSuite) TestGetStats() {
	tests := []struct {
		name       string
		query      string
		boundaryID int
		cached     bool
		err        error
		statusCode int
	}{
		{name: "Ok200", statusCode: http.StatusOK},
		{name: "Ok200Boundary", query: "?boundary_id=2", boundaryID: 2, statusCode: http.StatusOK},
		{name: "Ok200Cached", cached: true, statusCode: http.StatusOK},
		{name: "Err400Boundary", query: "?boundary_id=-1", statusCode: http.StatusBadRequest},
		{name: "Err400Usecase", err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err500", err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			cacheKey := mwcache.Key(http.MethodGet, "/open/stats"+tt.query, models.LangFromContext(suite.T().Context()))
			if tt.cached {
				body, err := json.Marshal(responses.Response[models.OpenStats]{Success: true, Payload: stats()})
				suite.Require().NoError(err)
				suite.cacher.On("GetBytes", mock.Anything, cacheKey).Once().Return(body, nil)
			} else {
				suite.cacher.On("GetBytes", mock.Anything, cacheKey).Once().Return(nil, errors.New("redis: nil"))
				if tt.statusCode != http.StatusBadRequest || tt.err != nil {
					suite.uc.On("GetOpenStats", mock.Anything, tt.boundaryID).Once().Return(stats(), tt.err)
				}
				if tt.statusCode == http.StatusOK {
					suite.cacher.On("Set", mock.Anything, cacheKey, mock.Anything, openrest.StatsCacheTTL).Once().Return(nil)
				}
			}

			req := httptest.NewRequest(http.MethodGet, "/open/stats"+tt.query, nil)
			w := httptest.NewRecorder()
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[models.OpenStats]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal(stats(), resp.Payload)
			}
		})
	}
}
