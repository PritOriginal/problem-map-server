package maprest_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/gin-gonic/gin"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/twpayne/go-geom"
)

type MapSuite struct {
	suite.Suite
	r      *gin.Engine
	uc     *maprest.MockMap
	cacher *mwcache.MockCacher
}

func (suite *MapSuite) SetupTest() {
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

func (suite *MapSuite) TestGetAdminBoundariesMarksCount_Filters() {
	tests := []struct {
		name        string
		query       string
		wantFilters *models.GetAdminBoundaryMarksCountFilters
		errUsecase  error
		statusCode  int
	}{
		{
			name:  "Ok200AllFilters",
			query: "?admin_levels=8&mark_type_ids=1,2&mark_status_ids=2,5&from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z",
			wantFilters: &models.GetAdminBoundaryMarksCountFilters{
				AdminLevels: []int{8}, MarkTypeIds: []int{1, 2}, MarkStatusIds: []int{2, 5},
				DateRange: models.DateRange{
					From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					To:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			statusCode: http.StatusOK,
		},
		{
			name:        "Ok200StatusOnly",
			query:       "?mark_status_ids=1",
			wantFilters: &models.GetAdminBoundaryMarksCountFilters{AdminLevels: []int{}, MarkTypeIds: []int{}, MarkStatusIds: []int{1}},
			statusCode:  http.StatusOK,
		},
		{name: "Err400StatusIds", query: "?mark_status_ids=a", statusCode: http.StatusBadRequest},
		{name: "Err400From", query: "?from=2026-01-01", statusCode: http.StatusBadRequest},
		{name: "Err400To", query: "?to=x", statusCode: http.StatusBadRequest},
		{
			name:        "Err400Usecase",
			query:       "?from=2026-02-01T00:00:00Z&to=2026-01-01T00:00:00Z",
			wantFilters: &models.GetAdminBoundaryMarksCountFilters{},
			errUsecase:  fmt.Errorf("op: %w", usecase.ErrInvalidArgument),
			statusCode:  http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantFilters != nil {
				var matcher any = mock.Anything
				if tt.errUsecase == nil {
					matcher = *tt.wantFilters
				}
				suite.uc.On("GetAdminBoundariesMarksCount", mock.Anything, matcher).Once().
					Return([]models.AdminBoundaryMarksCount{}, tt.errUsecase)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/map/admin-boundaries/marks/count"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *MapSuite) TestGetHeatmap() {
	const bbox = "?bbox=41.39,52.69,41.42,52.71"

	tests := []struct {
		name          string
		query         string
		wantCall      bool
		errGetHeatmap error
		statusCode    int
		wantMessage   string
	}{
		{name: "Ok200", query: bbox, wantCall: true, statusCode: http.StatusOK},
		{name: "Ok200Filters", query: bbox + "&cell_m=100&mark_type_ids=1,2&mark_status_ids=2", wantCall: true, statusCode: http.StatusOK},
		{name: "Err400NoBBox", query: "", statusCode: http.StatusBadRequest},
		{name: "Err400BadBBox", query: "?bbox=1,2,3", statusCode: http.StatusBadRequest},
		{name: "Err400InvertedBBox", query: "?bbox=41.42,52.71,41.39,52.69", statusCode: http.StatusBadRequest},
		{name: "Err400NegativeCell", query: bbox + "&cell_m=-5", statusCode: http.StatusBadRequest},
		{name: "Err400CellNotANumber", query: bbox + "&cell_m=big", statusCode: http.StatusBadRequest},
		{name: "Err400TypeIds", query: bbox + "&mark_type_ids=a", statusCode: http.StatusBadRequest},
		{name: "Err400StatusIds", query: bbox + "&mark_status_ids=a", statusCode: http.StatusBadRequest},
		{
			name: "Err400TooManyCells", query: bbox + "&cell_m=10", wantCall: true,
			errGetHeatmap: fmt.Errorf("op: %w", usecase.ErrTooManyHeatmapCells),
			statusCode:    http.StatusBadRequest, wantMessage: maprest.MsgTooManyCells,
		},
		{
			name: "Err400Invalid", query: bbox, wantCall: true,
			errGetHeatmap: fmt.Errorf("op: %w", usecase.ErrInvalidArgument),
			statusCode:    http.StatusBadRequest,
		},
		{name: "Err500", query: bbox, wantCall: true, errGetHeatmap: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cacher.
				On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().
				Return([]byte{}, errors.New(""))
			if tt.statusCode >= 200 && tt.statusCode < 300 {
				suite.cacher.
					On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, maprest.HeatmapCacheTTL).Once().
					Return(nil)
			}
			if tt.wantCall {
				suite.uc.On("GetHeatmap", mock.Anything, mock.Anything).Once().
					Return([]models.HeatmapCell{}, tt.errGetHeatmap)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/map/heatmap"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.wantMessage != "" {
				suite.Contains(w.Body.String(), tt.wantMessage)
			}
		})
	}
}

func (suite *MapSuite) TestGetHeatmap_GeoJSON() {
	suite.cacher.On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().Return([]byte{}, errors.New(""))
	suite.cacher.On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Once().Return(nil)

	hexagon := models.NewPolygon([][]geom.Coord{{
		{41.40, 52.70}, {41.41, 52.70}, {41.415, 52.705}, {41.41, 52.71}, {41.40, 52.71}, {41.395, 52.705}, {41.40, 52.70},
	}})
	suite.uc.On("GetHeatmap", mock.Anything, mock.Anything).Once().
		Return([]models.HeatmapCell{{Geom: hexagon, Count: 3}}, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/map/heatmap?bbox=41.39,52.69,41.42,52.71", nil)
	suite.r.ServeHTTP(w, req)

	handlertest.AssertResponse(suite.T(), w, http.StatusOK)

	var body struct {
		Payload struct {
			Type     string `json:"type"`
			Features []struct {
				Type     string `json:"type"`
				Geometry struct {
					Type        string        `json:"type"`
					Coordinates [][][]float64 `json:"coordinates"`
				} `json:"geometry"`
				Properties struct {
					Count int `json:"count"`
				} `json:"properties"`
			} `json:"features"`
		} `json:"payload"`
	}
	suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body))
	suite.Equal("FeatureCollection", body.Payload.Type)
	suite.Require().Len(body.Payload.Features, 1)
	f := body.Payload.Features[0]
	suite.Equal("Feature", f.Type)
	suite.Equal("Polygon", f.Geometry.Type)
	suite.Require().Len(f.Geometry.Coordinates, 1)
	suite.Len(f.Geometry.Coordinates[0], 7)
	suite.Equal(3, f.Properties.Count)
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}
