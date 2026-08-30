package syncrest_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	syncrest "github.com/PritOriginal/problem-map-server/internal/handler/sync"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	testKey    = "1234"
	testUserID = 7
)

type SyncSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *syncrest.MockSync
}

func (suite *SyncSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(testKey)})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.uc = syncrest.NewMockSync(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	syncrest.Register(suite.r, slogdiscard.NewDiscardLogger(), authMiddleware, suite.uc)
}

func TestSync(t *testing.T) {
	suite.Run(t, new(SyncSuite))
}

func (suite *SyncSuite) get(query string, auth bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/users/me/sync"+query, nil)
	if auth {
		accessToken, err := token.CreateToken(time.Minute, testUserID, string(models.RoleUser), testKey)
		suite.Require().NoError(err)
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, req)
	return w
}

func (suite *SyncSuite) TestGetUserSync() {
	since := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	serverTime := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		query       string
		auth        bool
		wantFilters models.UserSyncFilters
		sync        models.UserSync
		err         error
		statusCode  int
	}{
		{
			name:        "Ok200",
			query:       "?since=2025-03-01T12:00:00Z",
			auth:        true,
			wantFilters: models.UserSyncFilters{Since: since, Pagination: models.Pagination{Limit: models.DefaultLimit}},
			sync: models.UserSync{
				Tasks:         []models.Task{{ID: 1, UserID: testUserID}},
				Notifications: []models.Notification{{ID: 2}, {ID: 3}},
				Checks:        []models.Check{{ID: 4}},
				Totals:        models.UserSyncTotals{Tasks: 1, Notifications: 2, Checks: 1},
				ServerTime:    serverTime,
			},
			statusCode: http.StatusOK,
		},
		{
			name:        "Ok200EmptyArraysNotNull",
			query:       "?since=2025-03-01T12:00:00Z&limit=5&offset=10",
			auth:        true,
			wantFilters: models.UserSyncFilters{Since: since, Pagination: models.Pagination{Limit: 5, Offset: 10}},
			sync:        models.UserSync{ServerTime: serverTime},
			statusCode:  http.StatusOK,
		},
		{name: "Err401", query: "?since=2025-03-01T12:00:00Z", auth: false, statusCode: http.StatusUnauthorized},
		{name: "Err400MissingSince", query: "", auth: true, statusCode: http.StatusBadRequest},
		{
			name:        "Ok200SinceDateOnly",
			query:       "?since=2025-03-01",
			auth:        true,
			wantFilters: models.UserSyncFilters{Since: since.Truncate(24 * time.Hour), Pagination: models.Pagination{Limit: models.DefaultLimit}},
			sync:        models.UserSync{ServerTime: serverTime},
			statusCode:  http.StatusOK,
		},
		{name: "Err400SinceFormat", query: "?since=2025-03-01T12:00:00", auth: true, statusCode: http.StatusBadRequest},
		{name: "Err400SinceInFuture", query: "?since=2999-01-01T00:00:00Z", auth: true, statusCode: http.StatusBadRequest},
		{name: "Err400Limit", query: "?since=2025-03-01T12:00:00Z&limit=501", auth: true, statusCode: http.StatusBadRequest},
		{
			name:        "Err500",
			query:       "?since=2025-03-01T12:00:00Z",
			auth:        true,
			wantFilters: models.UserSyncFilters{Since: since, Pagination: models.Pagination{Limit: models.DefaultLimit}},
			err:         errors.New("db down"),
			statusCode:  http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode == http.StatusOK || tt.statusCode == http.StatusInternalServerError {
				suite.uc.On("GetUserSync", mock.Anything, testUserID, tt.wantFilters).Once().Return(tt.sync, tt.err)
			}

			w := suite.get(tt.query, tt.auth)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusOK {
				return
			}

			var resp responses.Response[syncrest.GetUserSyncResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Len(resp.Payload.Tasks, len(tt.sync.Tasks))
			suite.Len(resp.Payload.Notifications, len(tt.sync.Notifications))
			suite.Len(resp.Payload.Checks, len(tt.sync.Checks))
			suite.Equal(tt.sync.Totals, resp.Payload.Totals)
			suite.True(serverTime.Equal(resp.Payload.ServerTime))
			for _, field := range []string{`"tasks":[`, `"notifications":[`, `"checks":[`} {
				suite.Contains(w.Body.String(), field, "arrays, never null")
			}
		})
	}
}
