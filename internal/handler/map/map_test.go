package maprest_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/gin-gonic/gin"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MapSuite struct {
	suite.Suite
	r      *gin.Engine
	uc     *maprest.MockMap
	cacher *mwcache.MockCacher
}

func (suite *MapSuite) SetupSuite() {
	suite.uc = maprest.NewMockMap(suite.T())
	suite.cacher = mwcache.NewMockCacher(suite.T())

	log := slogdiscard.NewDiscardLogger()

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()

	maprest.Register(suite.r, log, suite.uc, suite.cacher)
}

func TestMap(t *testing.T) {
	suite.Run(t, new(MapSuite))
}

func (suite *MapSuite) TestGetAdminBoundaries() {
	tests := []struct {
		name                    string
		query                   string
		wantErrParseAdminLevels bool
		errGetAdminBoundaries   error
		statusCode              int
	}{
		{
			name:                  "Ok200",
			query:                 "",
			errGetAdminBoundaries: nil,
			statusCode:            http.StatusOK,
		},
		{
			name:                  "Ok200",
			query:                 "?admin_levels=9",
			errGetAdminBoundaries: nil,
			statusCode:            http.StatusOK,
		},
		{
			name:                  "Ok200",
			query:                 "?admin_levels=9,10",
			errGetAdminBoundaries: nil,
			statusCode:            http.StatusOK,
		},
		{
			name:                    "Err400",
			query:                   "?admin_levels=a",
			wantErrParseAdminLevels: true,
			statusCode:              http.StatusBadRequest,
		},
		{
			name:                  "Err500",
			query:                 "?admin_levels=9",
			errGetAdminBoundaries: errors.New(""),
			statusCode:            http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cacher.
				On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Once().
					Return(nil)
			}

			if !tt.wantErrParseAdminLevels {
				suite.uc.On("GetAdminBoundaries", mock.Anything, mock.Anything).Once().
					Return([]models.AdminBoundary{}, tt.errGetAdminBoundaries)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/map/admin-boundaries"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *MapSuite) TestGetAdminBoundariesMarksCount() {
	tests := []struct {
		name                            string
		query                           string
		wantErrParseAdminLevels         bool
		wantErrParseMarkTypeIds         bool
		errGetAdminBoundariesMarksCount error
		statusCode                      int
	}{
		{
			name:                            "Ok200",
			query:                           "",
			errGetAdminBoundariesMarksCount: nil,
			statusCode:                      http.StatusOK,
		},
		{
			name:                            "Ok200",
			query:                           "?admin_levels=9",
			errGetAdminBoundariesMarksCount: nil,
			statusCode:                      http.StatusOK,
		},
		{
			name:                            "Ok200",
			query:                           "?admin_levels=9,10",
			errGetAdminBoundariesMarksCount: nil,
			statusCode:                      http.StatusOK,
		},
		{
			name:                            "Ok200",
			query:                           "?admin_levels=9,10&mark_type_ids=",
			errGetAdminBoundariesMarksCount: nil,
			statusCode:                      http.StatusOK,
		},
		{
			name:                            "Ok200",
			query:                           "?admin_levels=9,10&mark_type_ids=1",
			errGetAdminBoundariesMarksCount: nil,
			statusCode:                      http.StatusOK,
		},
		{
			name:                            "Ok200",
			query:                           "?admin_levels=9,10&mark_type_ids=1,2",
			errGetAdminBoundariesMarksCount: nil,
			statusCode:                      http.StatusOK,
		},
		{
			name:                    "Err400",
			query:                   "?admin_levels=a",
			wantErrParseAdminLevels: true,
			statusCode:              http.StatusBadRequest,
		},
		{
			name:                    "Err400",
			query:                   "?mark_type_ids=a",
			wantErrParseAdminLevels: true,
			statusCode:              http.StatusBadRequest,
		},
		{
			name:                            "Err500",
			query:                           "?admin_levels=9",
			errGetAdminBoundariesMarksCount: errors.New(""),
			statusCode:                      http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseAdminLevels && !tt.wantErrParseMarkTypeIds {
				suite.uc.On("GetAdminBoundariesMarksCount", mock.Anything, mock.Anything).Once().
					Return([]models.AdminBoundaryMarksCount{}, tt.errGetAdminBoundariesMarksCount)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/map/admin-boundaries/marks/count"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *MapSuite) TestGetRegions() {
	tests := []struct {
		name          string
		errGetRegions error
		statusCode    int
	}{
		{
			name:          "Ok200",
			errGetRegions: nil,
			statusCode:    http.StatusOK,
		},
		{
			name:          "Err500",
			errGetRegions: errors.New(""),
			statusCode:    http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cacher.
				On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Once().
					Return(nil)
			}

			suite.uc.On("GetRegions", mock.Anything).Once().
				Return([]models.Region{}, tt.errGetRegions)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/map/regions", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *MapSuite) TestGetCities() {
	tests := []struct {
		name         string
		errGetCities error
		statusCode   int
	}{
		{
			name:         "Ok200",
			errGetCities: nil,
			statusCode:   200,
		},
		{
			name:         "Err500",
			errGetCities: errors.New(""),
			statusCode:   500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cacher.
				On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Once().
					Return(nil)
			}

			suite.uc.On("GetCities", mock.Anything).Once().
				Return([]models.City{}, tt.errGetCities)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/map/cities", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *MapSuite) TestGetDistricts() {
	tests := []struct {
		name            string
		errGetDistricts error
		statusCode      int
	}{
		{
			name:            "Ok200",
			errGetDistricts: nil,
			statusCode:      200,
		},
		{
			name:            "Err500",
			errGetDistricts: errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cacher.
				On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Once().
					Return(nil)
			}

			suite.uc.On("GetDistricts", mock.Anything).Once().
				Return([]models.District{}, tt.errGetDistricts)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/map/districts", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}
