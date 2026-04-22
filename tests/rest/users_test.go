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
}

func (st *UsersSuite) SetupSuite() {
	st.Cfg = config.MustLoadPath("../../configs/config.yaml")
}

func TestUsersSuite(t *testing.T) {
	suite.Run(t, new(UsersSuite))
}

func (st *UsersSuite) TestGetUsers() {
	response := getUsers(st.T(), &st.Cfg.REST, http.StatusOK)

	st.Equal(response.Success, true)
	st.NotNil(response.Payload.Users)
}

func getUsers(t *testing.T, cfg *config.RESTConfig, expectedStatusCode int) responses.Response[usersrest.GetUsersResponse] {
	resp, err := http.Get(fmt.Sprintf("http://%s:%d/users", cfg.Host, cfg.Port))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[usersrest.GetUsersResponse]

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *UsersSuite) TestGetUserById() {
	responseGetUsers := getUsers(st.T(), &st.Cfg.REST, http.StatusOK)

	tests := []struct {
		name           string
		id             string
		wantErrParseId bool
		errGetUserById error
		statusCode     int
	}{
		{
			name:       "Ok200",
			id:         strconv.Itoa(responseGetUsers.Payload.Users[0].Id),
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
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/users/%s", st.Cfg.REST.Host, st.Cfg.REST.Port, tt.id))
			st.NoError(err)
			defer resp.Body.Close()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[usersrest.GetUserByIdResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)

			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload.User)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}
