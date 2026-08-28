package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/middleware"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	gojwt "github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/suite"
)

const testKey = "1234"

type AuthSuite struct {
	suite.Suite
	r *gin.Engine
}

func (suite *AuthSuite) SetupSuite() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		Key: []byte(testKey),
	})
	if err != nil {
		panic(err)
	}
	if err := authMiddleware.MiddlewareInit(); err != nil {
		panic(err)
	}

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()

	auth := suite.r.Group("", authMiddleware.MiddlewareFunc())
	auth.GET("/me", func(c *gin.Context) {
		id, err := middleware.UserIDFromClaims(c)
		if err != nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": id, "role": middleware.RoleFromClaims(c)})
	})
	auth.GET("/moderation", middleware.RequireRole(models.RoleModerator, models.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}

func TestAuth(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

func (suite *AuthSuite) TestRequireRole() {
	tests := []struct {
		name       string
		role       string
		noToken    bool
		statusCode int
	}{
		{name: "Moderator", role: string(models.RoleModerator), statusCode: http.StatusOK},
		{name: "Admin", role: string(models.RoleAdmin), statusCode: http.StatusOK},
		{name: "User", role: string(models.RoleUser), statusCode: http.StatusForbidden},
		{name: "EmptyRole", role: "", statusCode: http.StatusForbidden},
		{name: "UnknownRole", role: "root", statusCode: http.StatusForbidden},
		{name: "NoToken", noToken: true, statusCode: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/moderation", nil)
			if !tt.noToken {
				accessToken, err := token.CreateToken(time.Minute, 1, tt.role, testKey)
				suite.NoError(err)
				req.Header.Set("Authorization", "Bearer "+accessToken)
			}

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

func (suite *AuthSuite) TestClaims() {
	accessToken, err := token.CreateToken(time.Minute, 42, string(models.RoleAdmin), testKey)
	suite.NoError(err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	suite.r.ServeHTTP(w, req)

	suite.Equal(http.StatusOK, w.Code)
	suite.JSONEq(`{"id":42,"role":"admin"}`, w.Body.String())
}

// signClaims issues a token with arbitrary claims to emulate legacy or
// malformed tokens.
func (suite *AuthSuite) signClaims(claims gojwt.MapClaims) string {
	claims["exp"] = time.Now().Add(time.Minute).Unix()
	signed, err := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims).SignedString([]byte(testKey))
	suite.Require().NoError(err)
	return signed
}

func (suite *AuthSuite) TestClaimsLegacyAndMalformed() {
	tests := []struct {
		name       string
		claims     gojwt.MapClaims
		statusCode int
		body       string
	}{
		{name: "NoRoleClaim", claims: gojwt.MapClaims{"sub": "7"}, statusCode: http.StatusOK, body: `{"id":7,"role":"user"}`},
		{name: "NonNumericSub", claims: gojwt.MapClaims{"sub": "abc", "role": "user"}, statusCode: http.StatusUnauthorized},
		{name: "NoSub", claims: gojwt.MapClaims{"role": "user"}, statusCode: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/me", nil)
			req.Header.Set("Authorization", "Bearer "+suite.signClaims(tt.claims))

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
			if tt.body != "" {
				suite.JSONEq(tt.body, w.Body.String())
			}
		})
	}
}
