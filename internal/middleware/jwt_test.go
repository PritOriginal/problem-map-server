package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/auth"
	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type JWTSuite struct {
	suite.Suite
	versions *auth.MockVersionChecker
	r        *gin.Engine
}

func TestJWT(t *testing.T) {
	suite.Run(t, new(JWTSuite))
}

func (suite *JWTSuite) SetupTest() {
	suite.versions = auth.NewMockVersionChecker(suite.T())
	suite.r = suite.router(middleware.JWTParams{Key: testKey, Versions: suite.versions})
}

func (suite *JWTSuite) router(p middleware.JWTParams) *gin.Engine {
	authMiddleware, err := middleware.NewJWT(slogdiscard.NewDiscardLogger(), p)
	suite.Require().NoError(err)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/me", authMiddleware.MiddlewareFunc(), func(c *gin.Context) {
		id, err := middleware.UserIDFromClaims(c)
		suite.Require().NoError(err)
		c.JSON(http.StatusOK, gin.H{"id": id, "role": middleware.RoleFromClaims(c)})
	})
	return r
}

func (suite *JWTSuite) do(r *gin.Engine, tokenString string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	if tokenString != "" {
		req.Header.Set("Authorization", "Bearer "+tokenString)
	}
	r.ServeHTTP(w, req)
	return w
}

func (suite *JWTSuite) issue(typ string, version int64, key string) string {
	tok, err := token.Create(token.Params{
		TTL: time.Minute, UserID: 7, Role: string(models.RoleAdmin), Type: typ, Version: version, ID: "jti",
	}, key)
	suite.Require().NoError(err)
	return tok
}

func (suite *JWTSuite) TestMiddleware() {
	errStore := errors.New("redis down")

	tests := []struct {
		name       string
		token      string
		version    *int64
		versionErr error
		statusCode int
		message    string
	}{
		{name: "Ok", token: suite.issue(token.TypeAccess, 2, testKey), version: ptr(int64(2)), statusCode: http.StatusOK},
		{name: "OkVersionZeroNoEntry", token: suite.issue(token.TypeAccess, 0, testKey), version: ptr(int64(0)), statusCode: http.StatusOK},
		{name: "OkStoreUnavailableFailsOpen", token: suite.issue(token.TypeAccess, 1, testKey), versionErr: errStore, statusCode: http.StatusOK},
		{name: "Err401VersionBehind", token: suite.issue(token.TypeAccess, 1, testKey), version: ptr(int64(2)), statusCode: http.StatusUnauthorized, message: "token revoked"},
		{name: "Err401VersionAhead", token: suite.issue(token.TypeAccess, 3, testKey), version: ptr(int64(2)), statusCode: http.StatusUnauthorized, message: "token revoked"},
		{name: "Err401RefreshToken", token: suite.issue(token.TypeRefresh, 2, testKey), statusCode: http.StatusUnauthorized, message: "not an access token"},
		{name: "Err401LegacyNoType", token: suite.issue("", 2, testKey), statusCode: http.StatusUnauthorized, message: "not an access token"},
		{name: "Err401WrongKey", token: suite.issue(token.TypeAccess, 2, "other-key"), statusCode: http.StatusUnauthorized},
		{name: "Err401NoToken", statusCode: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			// Every case gets a fresh middleware so the version cache is empty.
			versions := auth.NewMockVersionChecker(suite.T())
			r := suite.router(middleware.JWTParams{Key: testKey, Versions: versions})
			if tt.version != nil || tt.versionErr != nil {
				var v int64
				if tt.version != nil {
					v = *tt.version
				}
				versions.On("AuthVersion", mock.Anything, 7).Once().Return(v, tt.versionErr)
			}

			w := suite.do(r, tt.token)

			suite.Equal(tt.statusCode, w.Code, w.Body.String())
			if tt.message != "" {
				suite.Contains(w.Body.String(), tt.message)
			}
			if tt.statusCode == http.StatusOK {
				suite.JSONEq(`{"id":7,"role":"admin"}`, w.Body.String())
			}
		})
	}
}

func (suite *JWTSuite) TestVersionCache() {
	// One lookup serves several requests within the TTL; errors are retried.
	suite.versions.On("AuthVersion", mock.Anything, 7).Once().Return(int64(0), errors.New("down"))
	suite.versions.On("AuthVersion", mock.Anything, 7).Once().Return(int64(1), nil)

	tok := suite.issue(token.TypeAccess, 1, testKey)
	for i := 0; i < 3; i++ {
		suite.Equal(http.StatusOK, suite.do(suite.r, tok).Code)
	}
}

func (suite *JWTSuite) TestVersionCacheExpires() {
	versions := auth.NewMockVersionChecker(suite.T())
	r := suite.router(middleware.JWTParams{Key: testKey, Versions: versions, VersionCacheTTL: 20 * time.Millisecond})
	versions.On("AuthVersion", mock.Anything, 7).Once().Return(int64(1), nil)
	versions.On("AuthVersion", mock.Anything, 7).Once().Return(int64(2), nil)

	tok := suite.issue(token.TypeAccess, 1, testKey)
	suite.Equal(http.StatusOK, suite.do(r, tok).Code)
	time.Sleep(30 * time.Millisecond)
	suite.Equal(http.StatusUnauthorized, suite.do(r, tok).Code, "bumped version is picked up after the cache expires")
}

func (suite *JWTSuite) TestVersionCacheSharedInvalidatesOnBump() {
	// The cache shared with the usecases forgets the user on IncrAuthVersion,
	// so a bump made by this process is seen by the next request.
	src := middleware.NewMockVersionSource(suite.T())
	cache := middleware.NewVersionCache(src, time.Hour)
	r := suite.router(middleware.JWTParams{Key: testKey, Versions: cache})
	src.On("AuthVersion", mock.Anything, 7).Once().Return(int64(1), nil)
	src.On("IncrAuthVersion", mock.Anything, 7).Once().Return(int64(2), nil)
	src.On("AuthVersion", mock.Anything, 7).Once().Return(int64(2), nil)

	tok := suite.issue(token.TypeAccess, 1, testKey)
	suite.Equal(http.StatusOK, suite.do(r, tok).Code)
	suite.Equal(http.StatusOK, suite.do(r, tok).Code, "served from the cache")

	v, err := cache.IncrAuthVersion(context.Background(), 7)
	suite.Require().NoError(err)
	suite.Equal(int64(2), v)
	suite.Equal(http.StatusUnauthorized, suite.do(r, tok).Code, "bump is visible at once")
	suite.Equal(http.StatusUnauthorized, suite.do(r, tok).Code, "and cached again")
}

func (suite *JWTSuite) TestVersionCacheInvalidateOnBumpError() {
	src := middleware.NewMockVersionSource(suite.T())
	cache := middleware.NewVersionCache(src, time.Hour)
	src.On("AuthVersion", mock.Anything, 7).Once().Return(int64(1), nil)
	src.On("IncrAuthVersion", mock.Anything, 7).Once().Return(int64(0), errors.New("down"))
	src.On("AuthVersion", mock.Anything, 7).Once().Return(int64(1), nil)

	v, err := cache.AuthVersion(context.Background(), 7)
	suite.Require().NoError(err)
	suite.Equal(int64(1), v)
	_, err = cache.IncrAuthVersion(context.Background(), 7)
	suite.Error(err)
	_, err = cache.AuthVersion(context.Background(), 7)
	suite.NoError(err, "the entry was dropped and refetched")
}

func (suite *JWTSuite) TestWithoutVersions() {
	r := suite.router(middleware.JWTParams{Key: testKey})

	suite.Equal(http.StatusOK, suite.do(r, suite.issue(token.TypeAccess, 5, testKey)).Code)
	suite.Equal(http.StatusUnauthorized, suite.do(r, suite.issue(token.TypeRefresh, 5, testKey)).Code)
}

func ptr[T any](v T) *T { return &v }
