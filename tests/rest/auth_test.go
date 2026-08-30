//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AuthSuite struct {
	suite.Suite
	Cfg *config.Config
	fx  *fixtures
}

func (st *AuthSuite) SetupSuite() {
	st.Cfg, st.fx = loadFixtures(st.T())
}

func TestAuthSuite(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

func (st *AuthSuite) TestSignUp() {
	username := gofakeit.FirstName()
	login := gofakeit.Username()
	password := gofakeit.Password(true, true, true, true, true, 10)
	homePoint := models.NewPoint(fixtureHomePoint)

	tests := []struct {
		name       string
		rawReq     string
		req        authrest.SignUpRequest
		statusCode int
	}{
		{
			name: "Ok201",
			req: authrest.SignUpRequest{
				Username:  username,
				Login:     login,
				Password:  password,
				HomePoint: homePoint,
			},
			statusCode: http.StatusCreated,
		},
		{
			name:       "Err400InvalidJSON",
			rawReq:     "{",
			statusCode: http.StatusBadRequest,
		},
		{
			name: "Err400InvalidReq",
			req: authrest.SignUpRequest{
				Username: "name",
				Login:    "username",
			},
			statusCode: http.StatusBadRequest,
		},
		{
			name: "Err400InvalidReq",
			req: authrest.SignUpRequest{
				Username:  "name",
				Login:     "username",
				HomePoint: homePoint,
			},
			statusCode: http.StatusBadRequest,
		},
		{
			name: "Err409",
			req: authrest.SignUpRequest{
				Username:  username,
				Login:     login,
				Password:  password,
				HomePoint: homePoint,
			},
			statusCode: http.StatusConflict,
		},
		{
			name: "Err409FixtureLogin",
			req: authrest.SignUpRequest{
				Username:  st.fx.user.Username,
				Login:     st.fx.user.Login,
				Password:  st.fx.user.Password,
				HomePoint: homePoint,
			},
			statusCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			var request *bytes.Buffer
			if tt.rawReq == "" {
				reqJSON, err := json.Marshal(tt.req)
				st.NoError(err)
				request = bytes.NewBuffer(reqJSON)
			} else {
				request = bytes.NewBuffer([]byte(tt.rawReq))
			}

			response := signUp(st.T(), request, &st.Cfg.REST, tt.statusCode)
			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotNil(response.Payload)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func signUp(t *testing.T, req io.Reader, cfg *config.RESTConfig, expectedStatusCode int) responses.Response[authrest.SignUpResponse] {
	resp, err := http.Post(
		makeUrl(makeUrlParams{
			host: cfg.Host,
			port: cfg.Port,
			path: "/auth/signup",
		}),
		"application/json",
		req,
	)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[authrest.SignUpResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *AuthSuite) TestSignIn() {
	login := st.fx.user.Login
	password := st.fx.user.Password

	tests := []struct {
		name       string
		rawReq     string
		req        authrest.SignInRequest
		statusCode int
	}{
		{
			name: "Ok200",
			req: authrest.SignInRequest{
				Login:    login,
				Password: password,
			},
			statusCode: 200,
		},
		{
			name:       "Err400InvalidJSON",
			rawReq:     "{",
			statusCode: 400,
		},
		{
			name: "Err400InvalidReq",
			req: authrest.SignInRequest{
				Login: "username",
			},
			statusCode: 400,
		},
		{
			name: "Err401WrongPassword",
			req: authrest.SignInRequest{
				Login:    login,
				Password: password + "x",
			},
			statusCode: http.StatusUnauthorized,
		},
		{
			name: "Err401UnknownLogin",
			req: authrest.SignInRequest{
				Login:    "no-such-user-" + login,
				Password: password,
			},
			statusCode: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		st.Run(tt.name, func() {
			var request *bytes.Buffer
			if tt.rawReq == "" {
				reqJSON, err := json.Marshal(tt.req)
				st.NoError(err)
				request = bytes.NewBuffer(reqJSON)
			} else {
				request = bytes.NewBuffer([]byte(tt.rawReq))
			}

			response := signIn(st.T(), request, &st.Cfg.REST, tt.statusCode)
			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotEmpty(response.Payload.AccessToken)
				st.NotEmpty(response.Payload.RefreshToken)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func signIn(t *testing.T, req io.Reader, cfg *config.RESTConfig, expectedStatusCode int) responses.Response[authrest.SignInResponse] {
	resp, err := http.Post(
		makeUrl(makeUrlParams{
			host: cfg.Host,
			port: cfg.Port,
			path: "/auth/signin",
		}),
		"application/json",
		req,
	)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, expectedStatusCode, resp.StatusCode)

	var response responses.Response[authrest.SignInResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func (st *AuthSuite) TestRefreshTokens() {
	signInReqJson, err := json.Marshal(authrest.SignInRequest{
		Login:    st.fx.user.Login,
		Password: st.fx.user.Password,
	})
	st.Require().NoError(err)

	signInResponse := signIn(st.T(), bytes.NewBuffer(signInReqJson), &st.Cfg.REST, http.StatusOK)

	tests := []struct {
		name       string
		rawReq     string
		req        authrest.RefreshTokensRequest
		statusCode int
	}{
		{
			name: "Ok200",
			req: authrest.RefreshTokensRequest{
				RefreshToken: signInResponse.Payload.RefreshToken,
			},
			statusCode: http.StatusOK,
		},
		{
			name:       "Err400InvalidJSON",
			rawReq:     "{",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Err400InvalidReq-EmptyToken",
			req:        authrest.RefreshTokensRequest{},
			statusCode: http.StatusBadRequest,
		},
		{
			name: "Err400InvalidReq-InvalidToken",
			req: authrest.RefreshTokensRequest{
				RefreshToken: "abc",
			},
			statusCode: http.StatusBadRequest,
		},
		{
			name: "Err401",
			req: authrest.RefreshTokensRequest{
				RefreshToken: "a.b.c",
			},
			statusCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		st.Run(tt.name, func() {
			var request *bytes.Buffer
			if tt.rawReq == "" {
				reqJSON, err := json.Marshal(tt.req)
				st.NoError(err)
				request = bytes.NewBuffer(reqJSON)
			} else {
				request = bytes.NewBuffer([]byte(tt.rawReq))
			}

			resp, err := http.Post(
				makeUrl(makeUrlParams{
					host: st.Cfg.REST.Host,
					port: st.Cfg.REST.Port,
					path: "/auth/tokens/refresh",
				}),
				"application/json",
				request,
			)
			st.NoError(err)
			defer func() { _ = resp.Body.Close() }()

			st.Equal(tt.statusCode, resp.StatusCode)

			var response responses.Response[authrest.RefreshTokensResponse]
			err = json.NewDecoder(resp.Body).Decode(&response)
			st.NoError(err)
			if tt.statusCode < 300 {
				st.Equal(response.Success, true)
				st.NotEmpty(response.Payload.AccessToken)
				st.NotEmpty(response.Payload.RefreshToken)
			} else {
				st.Equal(response.Success, false)
			}
		})
	}
}

func addNewUser(t *testing.T, cfg *config.RESTConfig) responses.Response[authrest.SignInResponse] {
	username := gofakeit.FirstName()
	login := gofakeit.Username()
	password := gofakeit.Password(true, true, true, true, true, 10)

	signUpReqJSON, err := json.Marshal(authrest.SignUpRequest{
		Username:  username,
		Login:     login,
		Password:  password,
		HomePoint: models.NewPoint(fixtureHomePoint),
	})
	require.NoError(t, err)
	_ = signUp(t, bytes.NewBuffer(signUpReqJSON), cfg, http.StatusCreated)

	signInReqJson, err := json.Marshal(authrest.SignInRequest{
		Login:    login,
		Password: password,
	})
	require.NoError(t, err)
	return signIn(t, bytes.NewBuffer(signInReqJson), cfg, http.StatusOK)
}

// moderatorToken registers a new user and issues an access token with the
// moderator role for it, signed with the server access key from the config.
func moderatorToken(t *testing.T, cfg *config.Config) string {
	signInResponse := addNewUser(t, &cfg.REST)
	userId := currentUserId(t, &cfg.REST, signInResponse.Payload.AccessToken)

	accessToken, err := token.CreateToken(time.Hour, userId, string(models.RoleModerator), cfg.Auth.JWT.Access.Key)
	require.NoError(t, err)

	return accessToken
}
