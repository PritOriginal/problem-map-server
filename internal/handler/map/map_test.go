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
		{
			name:  "Ok200DateOnly",
			query: "?from=2026-01-01&to=2026-01-31",
			wantFilters: &models.GetAdminBoundaryMarksCountFilters{
				AdminLevels: []int{}, MarkTypeIds: []int{}, MarkStatusIds: []int{},
				DateRange: models.DateRange{
					From: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					To:   time.Date(2026, 1, 31, 23, 59, 59, 999999999, time.UTC),
				},
			},
			statusCode: http.StatusOK,
		},
		{name: "Err400From", query: "?from=2026-01-01T00:00:00", statusCode: http.StatusBadRequest},
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

// TestGetAdminBoundaries_Geometry checks the ?geometry= filter: it is passed
// down as GetAdminBoundaryFilters.WithGeometry and drops the geom field from
// the response.
func (suite *MapSuite) TestGetAdminBoundaries_Geometry() {
	boundary := models.AdminBoundary{
		Id: 1, Name: "Центр", AdminLevel: 8,
		Geom: models.NewMultiPolygon([][][]geom.Coord{{{{41.39, 52.69}, {41.42, 52.69}, {41.42, 52.71}, {41.39, 52.71}, {41.39, 52.69}}}}),
	}

	tests := []struct {
		name           string
		query          string
		wantFilters    *models.GetAdminBoundaryFilters
		wantGeomInBody bool
		statusCode     int
	}{
		{
			name:           "Ok200DefaultWithGeometry",
			query:          "",
			wantFilters:    &models.GetAdminBoundaryFilters{AdminLevels: []int{}, WithGeometry: true},
			wantGeomInBody: true,
			statusCode:     http.StatusOK,
		},
		{
			name:           "Ok200GeometryTrue",
			query:          "?geometry=true&admin_levels=8",
			wantFilters:    &models.GetAdminBoundaryFilters{AdminLevels: []int{8}, WithGeometry: true},
			wantGeomInBody: true,
			statusCode:     http.StatusOK,
		},
		{
			name:        "Ok200GeometryFalse",
			query:       "?geometry=false",
			wantFilters: &models.GetAdminBoundaryFilters{AdminLevels: []int{}, WithGeometry: false},
			statusCode:  http.StatusOK,
		},
		{name: "Err400BadGeometry", query: "?geometry=abc", statusCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.cacher.
				On("GetBytes", mock.Anything, mock.AnythingOfType("string")).Once().
				Return([]byte{}, errors.New(""))

			if tt.wantFilters != nil {
				suite.cacher.
					On("Set", mock.Anything, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Once().
					Return(nil)

				returned := boundary
				if !tt.wantFilters.WithGeometry {
					returned.Geom = nil
				}
				suite.uc.On("GetAdminBoundaries", mock.Anything, *tt.wantFilters).Once().
					Return([]models.AdminBoundary{returned}, nil)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/map/admin-boundaries"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.wantFilters == nil {
				return
			}

			var body struct {
				Payload struct {
					AdminBoundaries []map[string]json.RawMessage `json:"admin_boundaries"`
				} `json:"payload"`
			}
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &body))
			suite.Require().Len(body.Payload.AdminBoundaries, 1)
			_, ok := body.Payload.AdminBoundaries[0]["geom"]
			suite.Equal(tt.wantGeomInBody, ok, "body: %s", w.Body.String())
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

func (suite *MapSuite) TestGetAdminBoundaryGeoJSON() {
	boundary := models.AdminBoundary{
		Id: 5, Name: "Центр", AdminLevel: 8,
		Geom: models.NewMultiPolygon([][][]geom.Coord{{{{41.39, 52.69}, {41.42, 52.69}, {41.42, 52.71}, {41.39, 52.71}, {41.39, 52.69}}}}),
	}

	tests := []struct {
		name       string
		path       string
		id         int
		err        error
		statusCode int
	}{
		{name: "Ok200", path: "/map/admin-boundaries/5.geojson", id: 5, statusCode: http.StatusOK},
		{name: "Err404NotFound", path: "/map/admin-boundaries/5.geojson", id: 5, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", path: "/map/admin-boundaries/5.geojson", id: 5, err: errors.New("db down"), statusCode: http.StatusInternalServerError},
		{name: "Err400BadID", path: "/map/admin-boundaries/abc.geojson", statusCode: http.StatusBadRequest},
		{name: "Err400ZeroID", path: "/map/admin-boundaries/0.geojson", statusCode: http.StatusBadRequest},
		{name: "Err404OtherExtension", path: "/map/admin-boundaries/5.kml", statusCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.id != 0 {
				suite.uc.On("GetAdminBoundaryById", mock.Anything, tt.id).Once().Return(boundary, tt.err)
			}
			suite.cacher.On("GetBytes", mock.Anything, mwcache.Key("GET", tt.path, models.LangRU)).Once().
				Return(nil, errors.New("miss"))
			if tt.statusCode == http.StatusOK {
				suite.cacher.On("Set", mock.Anything, mwcache.Key("GET", tt.path, models.LangRU), mock.Anything, maprest.DictionaryCacheTTL).
					Once().Return(nil)
			}

			w := httptest.NewRecorder()
			suite.r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))

			suite.Equal(tt.statusCode, w.Code, w.Body.String())
			if tt.statusCode != http.StatusOK {
				handlertest.AssertResponse(suite.T(), w, tt.statusCode)
				suite.Empty(w.Header().Get("ETag"))
				return
			}
			suite.Equal(maprest.ContentTypeGeoJSON, w.Header().Get("Content-Type"))
			suite.Equal(mwcache.ETag(w.Body.Bytes()), w.Header().Get("ETag"))
			suite.Equal("public, max-age=86400", w.Header().Get("Cache-Control"))

			var feature struct {
				Type     string `json:"type"`
				ID       int    `json:"id"`
				Geometry struct {
					Type        string           `json:"type"`
					Coordinates [][][][2]float64 `json:"coordinates"`
				} `json:"geometry"`
				Properties maprest.AdminBoundaryProperties `json:"properties"`
			}
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &feature), w.Body.String())
			suite.Equal("Feature", feature.Type)
			suite.Equal(5, feature.ID)
			suite.Equal("MultiPolygon", feature.Geometry.Type)
			suite.Len(feature.Geometry.Coordinates[0][0], 5)
			suite.Equal(maprest.AdminBoundaryProperties{Name: "Центр", AdminLevel: 8}, feature.Properties)
		})
	}
}

// The static marks/count route must keep working next to the :file param.

func (suite *MapSuite) TestGetAdminBoundaryGeoJSON_NotModified() {
	boundary := models.AdminBoundary{
		Id: 5, Name: "Центр", AdminLevel: 8,
		Geom: models.NewMultiPolygon([][][]geom.Coord{{{{41.39, 52.69}, {41.42, 52.69}, {41.42, 52.71}, {41.39, 52.71}, {41.39, 52.69}}}}),
	}
	const path = "/map/admin-boundaries/5.geojson"
	key := mwcache.Key("GET", path, models.LangRU)

	// Fill the cache and remember what was stored.
	var saved []byte
	suite.uc.On("GetAdminBoundaryById", mock.Anything, 5).Once().Return(boundary, nil)
	suite.cacher.On("GetBytes", mock.Anything, key).Once().Return(nil, errors.New("miss"))
	suite.cacher.On("Set", mock.Anything, key, mock.Anything, maprest.DictionaryCacheTTL).Once().
		Run(func(args mock.Arguments) { saved = args.Get(2).([]byte) }).Return(nil)
	first := httptest.NewRecorder()
	suite.r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, path, nil))
	suite.Require().Equal(http.StatusOK, first.Code)
	etag := first.Header().Get("ETag")
	suite.Require().NotEmpty(etag)

	// A client presenting the ETag gets 304 straight from the cache.
	suite.cacher.On("GetBytes", mock.Anything, key).Once().Return(saved, nil)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	suite.r.ServeHTTP(second, req)

	suite.Equal(http.StatusNotModified, second.Code)
	suite.Empty(second.Body.Bytes())
	suite.Equal(etag, second.Header().Get("ETag"))

	// Without the validator the cached document is served with its content type.
	suite.cacher.On("GetBytes", mock.Anything, key).Once().Return(saved, nil)
	third := httptest.NewRecorder()
	suite.r.ServeHTTP(third, httptest.NewRequest(http.MethodGet, path, nil))
	suite.Equal(http.StatusOK, third.Code)
	suite.Equal(maprest.ContentTypeGeoJSON, third.Header().Get("Content-Type"))
	suite.JSONEq(first.Body.String(), third.Body.String())
}

func (suite *MapSuite) TestAdminBoundariesRoutesCoexist() {
	suite.uc.On("GetAdminBoundariesMarksCount", mock.Anything, mock.Anything).Once().Return([]models.AdminBoundaryMarksCount{}, nil)

	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/map/admin-boundaries/marks/count", nil))
	suite.Equal(http.StatusOK, w.Code, w.Body.String())
}
