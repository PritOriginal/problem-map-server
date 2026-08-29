//go:build functional && rest

package tests

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UsersSuite struct {
	suite.Suite
	Cfg *config.Config
	fx  *fixtures
}

func (st *UsersSuite) SetupSuite() {
	st.Cfg, st.fx = loadFixtures(st.T())
}

func TestUsersSuite(t *testing.T) {
	suite.Run(t, new(UsersSuite))
}

func (st *UsersSuite) TestGetUsers() {
	response := getUsers(st.T(), &st.Cfg.REST, http.StatusOK)

	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Users)

	st.Contains(ids(response.Payload.Users, func(u usersrest.PublicUser) int { return u.Id }), st.fx.user.ID, "fixture user must be listed")
}

func getUsers(t *testing.T, cfg *config.RESTConfig, expectedStatusCode int) responses.Response[usersrest.GetUsersResponse] {
	resp, err := http.Get(makeUrl(makeUrlParams{
		host: cfg.Host,
		port: cfg.Port,
		path: "/users",
	}))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[usersrest.GetUsersResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

// currentUserId returns the id of the user the access token belongs to (GET /users/me).
func currentUserId(t *testing.T, cfg *config.RESTConfig, accessToken string) int {
	req, err := http.NewRequest(http.MethodGet, makeUrl(makeUrlParams{
		host: cfg.Host,
		port: cfg.Port,
		path: "/users/me",
	}), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var response responses.Response[usersrest.GetMeResponse]
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&response))
	require.NotEmpty(t, response.Payload.User.Login)

	return response.Payload.User.Id
}

func (st *UsersSuite) TestGetUserById() {
	tests := []struct {
		name           string
		id             string
		wantErrParseId bool
		errGetUserById error
		statusCode     int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(st.fx.user.ID),
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400",
			id:         "a",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Err404",
			id:         strconv.Itoa(math.MaxInt32),
			statusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			resp, err := http.Get(makeUrl(makeUrlParams{
				host: st.Cfg.REST.Host,
				port: st.Cfg.REST.Port,
				path: fmt.Sprintf("/users/%s", tt.id),
			}))
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[usersrest.GetUserByIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.Require().NotNil(response.Payload.User)
				st.Equal(st.fx.user.ID, response.Payload.User.Id)
				st.Equal(st.fx.user.Username, response.Payload.User.Name)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}
