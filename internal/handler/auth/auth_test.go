package authrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type AuthSuite struct {
	suite.Suite
	r  *chi.Mux
	uc *authrest.MockAuth
}

func (suite *AuthSuite) SetupSuite() {
	suite.uc = authrest.NewMockAuth(suite.T())

	log := slogdiscard.NewDiscardLogger()
	validate := validator.New()
	baseHandler := &handlers.BaseHandler{Log: log, Validate: validate}

	suite.r = chi.NewRouter()

	authrest.Register(suite.r, suite.uc, baseHandler)
}

func TestMark(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

func (suite *AuthSuite) TestSignUp() {
	tests := []struct {
		name            string
		rawReq          string
		req             authrest.SignUpRequest
		wantErrParseReq bool
		errSignUp       error
		statusCode      int
	}{
		{
			name: "Ok201",
			req: authrest.SignUpRequest{
				Username: "name",
				Login:    "username",
				Password: "password",
			},
			wantErrParseReq: false,
			errSignUp:       nil,
			statusCode:      201,
		},
		{
			name:            "Err400InvalidJSON",
			rawReq:          "{",
			wantErrParseReq: true,
			errSignUp:       nil,
			statusCode:      400,
		},
		{
			name: "Err400InvalidReq",
			req: authrest.SignUpRequest{
				Username: "name",
				Login:    "username",
			},
			wantErrParseReq: true,
			errSignUp:       nil,
			statusCode:      400,
		},
		{
			name: "Err409",
			req: authrest.SignUpRequest{
				Username: "name",
				Login:    "username",
				Password: "password",
			},
			wantErrParseReq: false,
			errSignUp:       usecase.ErrConflict,
			statusCode:      409,
		},
		{
			name: "Err500",
			req: authrest.SignUpRequest{
				Username: "name",
				Login:    "username",
				Password: "password",
			},
			wantErrParseReq: false,
			errSignUp:       errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseReq {
				suite.uc.On("SignUp", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Once().
					Return(int64(1), tt.errSignUp)
			}

			w := httptest.NewRecorder()

			var buf *bytes.Buffer
			if tt.rawReq == "" {
				body, err := json.Marshal(tt.req)
				suite.NoError(err)
				buf = bytes.NewBuffer(body)
			} else {
				buf = bytes.NewBuffer([]byte(tt.rawReq))
			}

			req := httptest.NewRequest("POST", "/auth/signup", buf)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *AuthSuite) TestSignIn() {
	tests := []struct {
		name            string
		rawReq          string
		req             authrest.SignInRequest
		wantErrParseReq bool
		errSignIn       error
		statusCode      int
	}{
		{
			name: "Ok200",
			req: authrest.SignInRequest{
				Login:    "username",
				Password: "password",
			},
			wantErrParseReq: false,
			errSignIn:       nil,
			statusCode:      200,
		},
		{
			name:            "Err400InvalidJSON",
			rawReq:          "{",
			wantErrParseReq: true,
			errSignIn:       nil,
			statusCode:      400,
		},
		{
			name: "Err400InvalidReq",
			req: authrest.SignInRequest{
				Login: "username",
			},
			wantErrParseReq: true,
			errSignIn:       nil,
			statusCode:      400,
		},
		{
			name: "Err409",
			req: authrest.SignInRequest{
				Login:    "username",
				Password: "password",
			},
			wantErrParseReq: false,
			errSignIn:       storage.ErrNotFound,
			statusCode:      401,
		},
		{
			name: "Err500",
			req: authrest.SignInRequest{
				Login:    "username",
				Password: "password",
			},
			wantErrParseReq: false,
			errSignIn:       errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseReq {
				suite.uc.On("SignIn", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Once().
					Return("accessToken", "refreshToken", tt.errSignIn)
			}

			w := httptest.NewRecorder()

			var buf *bytes.Buffer
			if tt.rawReq == "" {
				body, err := json.Marshal(tt.req)
				suite.NoError(err)
				buf = bytes.NewBuffer(body)
			} else {
				buf = bytes.NewBuffer([]byte(tt.rawReq))
			}

			req := httptest.NewRequest("POST", "/auth/signin", buf)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *AuthSuite) TestRefreshTokens() {
	tests := []struct {
		name            string
		rawReq          string
		req             authrest.RefreshTokensRequest
		wantErrParseReq bool
		errSignIn       error
		statusCode      int
	}{
		{
			name: "Ok200",
			req: authrest.RefreshTokensRequest{
				RefreshToken: "a.b.c",
			},
			wantErrParseReq: false,
			errSignIn:       nil,
			statusCode:      200,
		},
		{
			name:            "Err400InvalidJSON",
			rawReq:          "{",
			wantErrParseReq: true,
			errSignIn:       nil,
			statusCode:      400,
		},
		{
			name:            "Err400InvalidReq-EmptyToken",
			req:             authrest.RefreshTokensRequest{},
			wantErrParseReq: true,
			errSignIn:       nil,
			statusCode:      400,
		},
		{
			name: "Err400InvalidReq-InvalidToken",
			req: authrest.RefreshTokensRequest{
				RefreshToken: "abc",
			},
			wantErrParseReq: true,
			errSignIn:       nil,
			statusCode:      400,
		},
		{
			name: "Err409",
			req: authrest.RefreshTokensRequest{
				RefreshToken: "a.b.c",
			},
			wantErrParseReq: false,
			errSignIn:       storage.ErrNotFound,
			statusCode:      401,
		},
		{
			name: "Err500",
			req: authrest.RefreshTokensRequest{
				RefreshToken: "a.b.c",
			},
			wantErrParseReq: false,
			errSignIn:       errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseReq {
				suite.uc.On("RefreshTokens", mock.Anything, mock.AnythingOfType("string")).Once().
					Return("accessToken", "refreshToken", tt.errSignIn)
			}

			w := httptest.NewRecorder()

			var buf *bytes.Buffer
			if tt.rawReq == "" {
				body, err := json.Marshal(tt.req)
				suite.NoError(err)
				buf = bytes.NewBuffer(body)
			} else {
				buf = bytes.NewBuffer([]byte(tt.rawReq))
			}

			req := httptest.NewRequest("POST", "/auth/tokens/refresh", buf)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}
