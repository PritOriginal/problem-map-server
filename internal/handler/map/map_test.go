package maprest_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	mwcache "github.com/PritOriginal/problem-map-server/internal/middleware/cache"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type MapSuite struct {
	suite.Suite
	r      *chi.Mux
	uc     *maprest.MockMap
	cacher *mwcache.MockCacher
}

func (suite *MapSuite) SetupSuite() {
	suite.uc = maprest.NewMockMap(suite.T())
	suite.cacher = mwcache.NewMockCacher(suite.T())

	log := slogdiscard.NewDiscardLogger()
	validate := validator.New()
	baseHandler := &handlers.BaseHandler{Log: log, Validate: validate}

	suite.r = chi.NewRouter()

	maprest.Register(suite.r, suite.uc, suite.cacher, baseHandler)
}

func TestMark(t *testing.T) {
	suite.Run(t, new(MapSuite))
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
