//go:build functional

package tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/PritOriginal/problem-map-server/tests/rest/suite"
	"github.com/stretchr/testify/require"
)

func TestGetUsers(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/users", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp.Body.Close()
}
