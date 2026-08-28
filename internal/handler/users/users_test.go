package usersrest_test

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type UsersSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *usersrest.MockUsers
}

func (suite *UsersSuite) SetupSuite() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		Key: []byte("1234"),
	})
	if err != nil {
		panic(err)
	}
	if err := authMiddleware.MiddlewareInit(); err != nil {
		panic(err)
	}

	suite.uc = usersrest.NewMockUsers(suite.T())

	log := slogdiscard.NewDiscardLogger()

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()

	usersrest.Register(suite.r, log, authMiddleware, suite.uc)
}

func testUser() models.User {
	return models.User{
		Id:        1,
		Name:      "name",
		Login:     "login",
		HomePoint: models.NewPoint([]float64{1, 2}),
		Rating:    3,
		Role:      models.RoleUser,
	}
}

func TestUsers(t *testing.T) {
	suite.Run(t, new(UsersSuite))
}

func (suite *UsersSuite) TestGetUserById() {
	tests := []struct {
		name           string
		id             string
		wantErrParseId bool
		errGetUserById error
		statusCode     int
	}{
		{
			name:           "Ok200",
			id:             "1",
			wantErrParseId: false,
			errGetUserById: nil,
			statusCode:     200,
		},
		{
			name:           "Err400",
			id:             "a",
			wantErrParseId: true,
			errGetUserById: nil,
			statusCode:     400,
		},
		{
			name:           "Err404",
			id:             "1",
			wantErrParseId: false,
			errGetUserById: repository.ErrNotFound,
			statusCode:     404,
		},
		{
			name:           "Err500",
			id:             "1",
			wantErrParseId: false,
			errGetUserById: errors.New(""),
			statusCode:     500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrParseId {
				suite.uc.On("GetUserById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(testUser(), tt.errGetUserById)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)

			if tt.statusCode == 200 {
				var resp responses.Response[map[string]map[string]any]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				user := resp.Payload["user"]
				suite.Equal("name", user["username"])
				suite.NotContains(user, "login")
				suite.NotContains(user, "home_point")
			}
		})
	}
}

func (suite *UsersSuite) TestGetMe() {
	tests := []struct {
		name           string
		noToken        bool
		errGetUserById error
		statusCode     int
	}{
		{
			name:       "Ok200",
			statusCode: 200,
		},
		{
			name:       "Err401",
			noToken:    true,
			statusCode: 401,
		},
		{
			name:           "Err404",
			errGetUserById: repository.ErrNotFound,
			statusCode:     404,
		},
		{
			name:           "Err500",
			errGetUserById: errors.New(""),
			statusCode:     500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.noToken {
				suite.uc.On("GetUserById", mock.Anything, 1).Once().
					Return(testUser(), tt.errGetUserById)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users/me", nil)
			if !tt.noToken {
				accessToken, err := token.CreateToken(1*time.Minute, 1, string(models.RoleUser), "1234")
				suite.NoError(err)
				req.Header.Set("Authorization", "Bearer "+accessToken)
			}

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)

			if tt.statusCode == 200 {
				var resp responses.Response[map[string]map[string]any]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				user := resp.Payload["user"]
				suite.Equal("login", user["login"])
				suite.Contains(user, "home_point")
			}
		})
	}
}

func (suite *UsersSuite) TestGetUsers() {
	tests := []struct {
		name        string
		errGetUsers error
		statusCode  int
	}{
		{
			name:        "Ok",
			errGetUsers: nil,
			statusCode:  200,
		},
		{
			name:        "Err",
			errGetUsers: errors.New(""),
			statusCode:  500,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.uc.On("GetUsers", mock.Anything).Once().
				Return([]models.User{testUser()}, tt.errGetUsers)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)

			if tt.statusCode == 200 {
				var resp responses.Response[map[string][]map[string]any]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Len(resp.Payload["users"], 1)
				suite.NotContains(resp.Payload["users"][0], "login")
				suite.NotContains(resp.Payload["users"][0], "home_point")
			}
		})
	}
}
