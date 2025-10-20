//go:build functional && rest

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/tests/rest/suite"
	"github.com/stretchr/testify/require"
)

func TestGetUsers(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/users", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[usersrest.GetUsersResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.Users)
}

func TestGetUserById(t *testing.T) {
	st := suite.New(t)

	resp, err := http.Get(fmt.Sprintf("http://%s:%d/users/1", st.Cfg.REST.Host, st.Cfg.REST.Port))

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[usersrest.GetUserByIdResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotNil(t, response.Payload.User)
}
