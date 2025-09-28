package tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/PritOriginal/problem-map-server/tests/rest/suite"
	"github.com/stretchr/testify/require"
)

func TestGetRegions(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/regions", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp.Body.Close()
}

func TestGetCities(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/cities", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp.Body.Close()
}

func TestGetDistricts(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/districts", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp.Body.Close()
}

func TestGetMarks(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/map/marks", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp.Body.Close()
}
