//go:build functional && rest

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/stretchr/testify/suite"
)

type MapSuite struct {
	suite.Suite
	Cfg *config.Config
}

func (st *MapSuite) SetupSuite() {
	st.Cfg = config.MustLoadPath("../../configs/config.yaml")
}

func TestMapSuite(t *testing.T) {
	suite.Run(t, new(MapSuite))
}

func (st *MapSuite) TestGetAdminBoundaries() {
	tests := []struct {
		name       string
		query      string
		statusCode int
	}{
		{
			name:       "Ok200",
			query:      "",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "admin_levels=9",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "admin_levels=9,10",
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400",
			query:      "admin_levels=a",
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host:  st.Cfg.REST.Host,
				port:  st.Cfg.REST.Port,
				path:  "/map/admin-boundaries",
				query: tt.query,
			}))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[maprest.GetAdminBoundariesResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.AdminBoundaries)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MapSuite) TestGetAdminBoundariesMarksCount() {
	tests := []struct {
		name       string
		query      string
		statusCode int
	}{
		{
			name:       "Ok200",
			query:      "",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "admin_levels=9",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "admin_levels=9,10",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "admin_levels=9,10&mark_type_ids=",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "admin_levels=9,10&mark_type_ids=1",
			statusCode: http.StatusOK,
		},
		{
			name:       "Ok200",
			query:      "admin_levels=9,10&mark_type_ids=1,2",
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400",
			query:      "admin_levels=a",
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host:  st.Cfg.REST.Host,
				port:  st.Cfg.REST.Port,
				path:  "/map/admin-boundaries/marks/count",
				query: tt.query,
			}))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[maprest.GetAdminBoundariesMarksCountResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.AdminBoundaries)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func (st *MapSuite) TestGetRegions() {
	resp, err := http.Get(makeUrl(makeUrlParams{
		host: st.Cfg.REST.Host,
		port: st.Cfg.REST.Port,
		path: "/map/regions",
	}))

	st.NoError(err)
	st.Equal(http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.Response[maprest.GetRegionsResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	st.NoError(err)
	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Regions)
}

func (st *MapSuite) TestGetCities() {
	resp, err := http.Get(makeUrl(makeUrlParams{
		host: st.Cfg.REST.Host,
		port: st.Cfg.REST.Port,
		path: "/map/cities",
	}))

	st.NoError(err)
	st.Equal(http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.Response[maprest.GetCitiesResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	st.NoError(err)
	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Cities)
}

func (st *MapSuite) TestGetDistricts() {
	resp, err := http.Get(makeUrl(makeUrlParams{
		host: st.Cfg.REST.Host,
		port: st.Cfg.REST.Port,
		path: "/map/districts",
	}))

	st.NoError(err)
	st.Equal(http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.Response[maprest.GetDistrictsResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	st.NoError(err)
	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Districts)
}
