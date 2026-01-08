//go:build functional && rest

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/tests/rest/suite"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
)

func TestSignUp(t *testing.T) {
	st := suite.New(t)

	name := gofakeit.FirstName()
	username := gofakeit.Username()
	password := gofakeit.Password(true, true, true, true, true, 10)

	tests := []struct {
		testName string
		name     string
		username string
		password string
		httpCode int
	}{
		{
			testName: "successful",
			name:     name,
			username: username,
			password: password,
			httpCode: http.StatusCreated,
		},
		{
			testName: "user already exist",
			name:     name,
			username: username,
			password: password,
			httpCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {

			req := authrest.SignUpRequest{
				Name:     tt.name,
				Username: tt.username,
				Password: tt.password,
			}

			reqJSON, err := json.Marshal(req)
			require.NoError(t, err)

			resp, err := http.Post(
				fmt.Sprintf("http://%s:%d/auth/signup", st.Cfg.REST.Host, st.Cfg.REST.Port),
				"application/json",
				bytes.NewBuffer(reqJSON),
			)
			require.NoError(t, err)
			require.Equal(t, tt.httpCode, resp.StatusCode)

			defer resp.Body.Close()
		})
	}
}

func TestSignIn(t *testing.T) {
	st := suite.New(t)

	req := authrest.SignInRequest{
		Username: "user4",
		Password: "1234qwer",
	}

	response := signIn(t, st, req)
	require.NotEmpty(t, response.Payload.AccessToken)
	require.NotEmpty(t, response.Payload.RefreshToken)
}

func signIn(t *testing.T, st *suite.Suite, req authrest.SignInRequest) responses.SucceededResponse[authrest.SignInResponse] {
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(
		fmt.Sprintf("http://%s:%d/auth/signin", st.Cfg.REST.Host, st.Cfg.REST.Port),
		"application/json",
		bytes.NewBuffer(reqJSON),
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[authrest.SignInResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func TestRefreshTokens(t *testing.T) {
	st := suite.New(t)

	signInResponse := signIn(t, st, authrest.SignInRequest{
		Username: "user4",
		Password: "1234qwer",
	})

	req := authrest.RefreshTokensRequest{
		RefreshToken: signInResponse.Payload.RefreshToken,
	}

	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	resp, err := http.Post(
		fmt.Sprintf("http://%s:%d/auth/tokens/refresh", st.Cfg.REST.Host, st.Cfg.REST.Port),
		"application/json",
		bytes.NewBuffer(reqJSON),
	)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	defer resp.Body.Close()

	var response responses.SucceededResponse[authrest.RefreshTokensResponse]
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	require.NotEmpty(t, response.Payload.AccessToken)
	require.NotEmpty(t, response.Payload.RefreshToken)
}
