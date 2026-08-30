package authrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authrest "github.com/PritOriginal/problem-map-server/internal/handler/auth"
	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"github.com/twpayne/go-geom"
)

type AuthSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *authrest.MockAuth
}

func (suite *AuthSuite) SetupTest() {
	suite.uc = authrest.NewMockAuth(suite.T())

	log := slogdiscard.NewDiscardLogger()

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()

	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(testKey)})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	authrest.Register(suite.r, log, suite.uc, authMiddleware)
}

const testKey = "1234"

// bearer returns an Authorization header value for user 1.
func (suite *AuthSuite) bearer() string {
	accessToken, err := token.CreateToken(time.Minute, 1, "user", testKey)
	suite.Require().NoError(err)
	return "Bearer " + accessToken
}

func TestAuth(t *testing.T) {
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
				Username:  "name",
				Login:     "username",
				Password:  "password",
				HomePoint: models.NewPoint(geom.Coord{41.400422, 52.699787}),
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
				Password: "password",
			},
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
				Username:  "name",
				Login:     "username",
				Password:  "password",
				HomePoint: models.NewPoint(geom.Coord{41.400422, 52.699787}),
			},
			wantErrParseReq: false,
			errSignUp:       usecase.ErrConflict,
			statusCode:      409,
		},
		{
			name: "Err500",
			req: authrest.SignUpRequest{
				Username:  "name",
				Login:     "username",
				Password:  "password",
				HomePoint: models.NewPoint(geom.Coord{41.400422, 52.699787}),
			},
			wantErrParseReq: false,
			errSignUp:       errors.New(""),
			statusCode:      500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseReq {
				suite.uc.On("SignUp", mock.Anything, mock.AnythingOfType("usecase.SignUpParams")).Once().
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
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
			name: "Err401",
			req: authrest.SignInRequest{
				Login:    "username",
				Password: "password",
			},
			wantErrParseReq: false,
			errSignIn:       usecase.ErrUnauthorized,
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
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
			name: "Err401",
			req: authrest.RefreshTokensRequest{
				RefreshToken: "a.b.c",
			},
			wantErrParseReq: false,
			errSignIn:       usecase.ErrUnauthorized,
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *AuthSuite) TestLogout() {
	tests := []struct {
		name       string
		noToken    bool
		rawReq     string
		req        authrest.LogoutRequest
		wantUCCall bool
		errLogout  error
		statusCode int
	}{
		{name: "Ok204", req: authrest.LogoutRequest{RefreshToken: "a.b.c"}, wantUCCall: true, statusCode: http.StatusNoContent},
		{name: "Err401NoToken", noToken: true, req: authrest.LogoutRequest{RefreshToken: "a.b.c"}, statusCode: http.StatusUnauthorized},
		{name: "Err400InvalidJSON", rawReq: "{", statusCode: http.StatusBadRequest},
		{name: "Err400EmptyRefresh", req: authrest.LogoutRequest{}, statusCode: http.StatusBadRequest},
		{name: "Err400NotJWT", req: authrest.LogoutRequest{RefreshToken: "abc"}, statusCode: http.StatusBadRequest},
		{name: "Err401ForeignToken", req: authrest.LogoutRequest{RefreshToken: "a.b.c"}, wantUCCall: true, errLogout: usecase.ErrUnauthorized, statusCode: http.StatusUnauthorized},
		{name: "Err500", req: authrest.LogoutRequest{RefreshToken: "a.b.c"}, wantUCCall: true, errLogout: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantUCCall {
				suite.uc.On("Logout", mock.Anything, 1, tt.req.RefreshToken).Once().Return(tt.errLogout)
			}

			var buf *bytes.Buffer
			if tt.rawReq == "" {
				body, err := json.Marshal(tt.req)
				suite.NoError(err)
				buf = bytes.NewBuffer(body)
			} else {
				buf = bytes.NewBufferString(tt.rawReq)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/auth/logout", buf)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer())
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *AuthSuite) TestLogoutAll() {
	tests := []struct {
		name         string
		noToken      bool
		errLogoutAll error
		statusCode   int
	}{
		{name: "Ok204", statusCode: http.StatusNoContent},
		{name: "Err401NoToken", noToken: true, statusCode: http.StatusUnauthorized},
		{name: "Err500", errLogoutAll: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.noToken {
				suite.uc.On("LogoutAll", mock.Anything, 1).Once().Return(tt.errLogoutAll)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/auth/logout-all", nil)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer())
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}
