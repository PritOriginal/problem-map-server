package apikeysrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apikeysrest "github.com/PritOriginal/problem-map-server/internal/handler/apikeys"
	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	testKey    = "1234"
	testUserID = 7
)

var errInternal = errors.New("boom")

type APIKeysSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *apikeysrest.MockAPIKeys
}

func (suite *APIKeysSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(testKey)})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.uc = apikeysrest.NewMockAPIKeys(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	apikeysrest.Register(suite.r, slogdiscard.NewDiscardLogger(), authMiddleware, suite.uc)
}

func TestAPIKeys(t *testing.T) {
	suite.Run(t, new(APIKeysSuite))
}

// do performs a request as role (empty role means anonymous).
func (suite *APIKeysSuite) do(method, path string, body []byte, role models.Role) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		accessToken, err := token.CreateToken(time.Minute, testUserID, string(role), testKey)
		suite.Require().NoError(err)
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, req)
	return w
}

func user() models.Actor { return models.Actor{UserID: testUserID, Role: models.RoleUser} }

func apiKey() models.APIKey {
	return models.APIKey{ID: 3, OwnerUserID: testUserID, Name: "dashboard", Prefix: "pm_live_01234567", Scopes: []string{"read"}, RateLimitPerMin: 600, Active: true}
}

func (suite *APIKeysSuite) TestCreateAPIKey() {
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		body        string
		role        models.Role
		wantName    string
		wantExpires null.Time
		err         error
		statusCode  int
	}{
		{name: "Created201", body: `{"name":"dashboard"}`, role: models.RoleUser, wantName: "dashboard", statusCode: http.StatusCreated},
		{name: "Created201Expires", body: `{"name":"dashboard","expires_at":"2027-01-01T00:00:00Z"}`, role: models.RoleUser, wantName: "dashboard", wantExpires: null.TimeFrom(expires), statusCode: http.StatusCreated},
		{name: "Anonymous401", body: `{"name":"dashboard"}`, statusCode: http.StatusUnauthorized},
		{name: "Err400NoName", body: `{}`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400BadJSON", body: `{`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400BadExpires", body: `{"name":"dashboard","expires_at":"tomorrow"}`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400Usecase", body: `{"name":"dashboard"}`, role: models.RoleUser, wantName: "dashboard", err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err500", body: `{"name":"dashboard"}`, role: models.RoleUser, wantName: "dashboard", err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantName != "" {
				suite.uc.On("Create", mock.Anything, user(), tt.wantName, tt.wantExpires).Once().
					Return(apiKey(), "pm_live_0123456789abcdef0123456789abcdef", tt.err)
			}

			w := suite.do(http.MethodPost, "/api-keys", []byte(tt.body), tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == http.StatusCreated {
				var resp responses.Response[apikeysrest.CreateAPIKeyResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal("pm_live_0123456789abcdef0123456789abcdef", resp.Payload.Key)
				suite.Equal(3, resp.Payload.APIKey.ID)
				suite.NotContains(w.Body.String(), "key_hash")
			}
		})
	}
}

func (suite *APIKeysSuite) TestGetAPIKeys() {
	tests := []struct {
		name       string
		query      string
		role       models.Role
		all        bool
		err        error
		statusCode int
	}{
		{name: "Own200", role: models.RoleUser, statusCode: http.StatusOK},
		{name: "AdminAll200", query: "?all=true", role: models.RoleAdmin, all: true, statusCode: http.StatusOK},
		{name: "UserAll403", query: "?all=true", role: models.RoleUser, all: true, err: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Anonymous401", statusCode: http.StatusUnauthorized},
		{name: "Err400Query", query: "?all=maybe", role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err500", role: models.RoleUser, err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.role != "" && tt.statusCode != http.StatusBadRequest {
				suite.uc.On("List", mock.Anything, models.Actor{UserID: testUserID, Role: tt.role}, tt.all).Once().
					Return([]models.APIKey{apiKey()}, tt.err)
			}

			w := suite.do(http.MethodGet, "/api-keys"+tt.query, nil, tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == http.StatusOK {
				var resp responses.Response[apikeysrest.GetAPIKeysResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Len(resp.Payload.APIKeys, 1)
			}
		})
	}
}

func (suite *APIKeysSuite) TestDeleteAPIKey() {
	tests := []struct {
		name       string
		path       string
		role       models.Role
		err        error
		statusCode int
	}{
		{name: "Revoked200", path: "/api-keys/3", role: models.RoleUser, statusCode: http.StatusOK},
		{name: "Anonymous401", path: "/api-keys/3", statusCode: http.StatusUnauthorized},
		{name: "Err400Id", path: "/api-keys/x", role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err403", path: "/api-keys/3", role: models.RoleUser, err: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err404", path: "/api-keys/3", role: models.RoleUser, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", path: "/api-keys/3", role: models.RoleUser, err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.role != "" && tt.statusCode != http.StatusBadRequest {
				suite.uc.On("Revoke", mock.Anything, user(), 3).Once().Return(tt.err)
			}

			w := suite.do(http.MethodDelete, tt.path, nil, tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == http.StatusOK {
				suite.Contains(w.Body.String(), `"api_key_id":3`)
			}
		})
	}
}
