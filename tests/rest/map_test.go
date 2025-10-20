//go:build functional && rest

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	maprest "github.com/PritOriginal/problem-map-server/internal/handler/map"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/tests/rest/suite"
	"github.com/stretchr/testify/require"
)

func TestGetRegions(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/regions", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[maprest.GetRegionsResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Regions)
}

func TestGetCities(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/cities", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[maprest.GetCitiesResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Cities)
}

func TestGetDistricts(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/districts", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[maprest.GetDistrictsResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Districts)
}

func TestGetMarks(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/marks", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[maprest.GetMarksResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Marks)
}

func TestAddMark(t *testing.T) {
	//TODO
}
