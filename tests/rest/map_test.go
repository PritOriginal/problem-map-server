//go:build functional && rest

package tests

import (
	"encoding/json"
	"fmt"
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

func (st *MapSuite) TestGetRegions() {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/regions", st.Cfg.REST.Host, st.Cfg.REST.Port))

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
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/cities", st.Cfg.REST.Host, st.Cfg.REST.Port))

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
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/districts", st.Cfg.REST.Host, st.Cfg.REST.Port))

	st.NoError(err)
	st.Equal(http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.Response[maprest.GetDistrictsResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	st.NoError(err)
	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Districts)
}
