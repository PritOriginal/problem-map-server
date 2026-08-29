package notificationsrest_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	notificationsrest "github.com/PritOriginal/problem-map-server/internal/handler/notifications"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
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

var errInternal = errors.New("boom")

type NotificationsSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *notificationsrest.MockNotifications
}

func (suite *NotificationsSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(testKey)})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.uc = notificationsrest.NewMockNotifications(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	notificationsrest.Register(suite.r, slogdiscard.NewDiscardLogger(), authMiddleware, suite.uc)
}

func TestNotifications(t *testing.T) {
	suite.Run(t, new(NotificationsSuite))
}

// do performs an authenticated request (unless auth is false) and returns the recorder.
func (suite *NotificationsSuite) do(method, path string, body []byte, auth bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		accessToken, err := token.CreateToken(time.Minute, testUserID, string(models.RoleUser), testKey)
		suite.Require().NoError(err)
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, req)
	return w
}

func (suite *NotificationsSuite) TestGetNotifications() {
	tests := []struct {
		name        string
		query       string
		auth        bool
		wantFilters *models.GetNotificationsFilters
		err         error
		statusCode  int
	}{
		{
			name:        "Ok200",
			auth:        true,
			wantFilters: &models.GetNotificationsFilters{Pagination: models.Pagination{Limit: 100}},
			statusCode:  http.StatusOK,
		},
		{
			name:        "Ok200Unread",
			query:       "?unread=true&limit=10&offset=5",
			auth:        true,
			wantFilters: &models.GetNotificationsFilters{UnreadOnly: true, Pagination: models.Pagination{Limit: 10, Offset: 5}},
			statusCode:  http.StatusOK,
		},
		{name: "Err400BadLimit", query: "?limit=0", auth: true, statusCode: http.StatusBadRequest},
		{name: "Err401", statusCode: http.StatusUnauthorized},
		{
			name:        "Err500",
			auth:        true,
			wantFilters: &models.GetNotificationsFilters{Pagination: models.Pagination{Limit: 100}},
			err:         errInternal,
			statusCode:  http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantFilters != nil {
				suite.uc.On("List", mock.Anything, testUserID, *tt.wantFilters).Once().
					Return(models.Page[models.Notification]{Items: []models.Notification{{ID: 1}}, Total: 1}, tt.err)
			}

			w := suite.do(http.MethodGet, "/notifications"+tt.query, nil, tt.auth)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *NotificationsSuite) TestGetUnreadCount() {
	tests := []struct {
		name       string
		auth       bool
		err        error
		statusCode int
	}{
		{name: "Ok200", auth: true, statusCode: http.StatusOK},
		{name: "Err401", statusCode: http.StatusUnauthorized},
		{name: "Err500", auth: true, err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.auth {
				suite.uc.On("UnreadCount", mock.Anything, testUserID).Once().Return(3, tt.err)
			}

			w := suite.do(http.MethodGet, "/notifications/unread-count", nil, tt.auth)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				suite.Contains(w.Body.String(), `"count":3`)
			}
		})
	}
}

func (suite *NotificationsSuite) TestMarkRead() {
	tests := []struct {
		name       string
		id         string
		auth       bool
		err        error
		statusCode int
	}{
		{name: "Ok200", id: "42", auth: true, statusCode: http.StatusOK},
		{name: "Err400", id: "abc", auth: true, statusCode: http.StatusBadRequest},
		{name: "Err401", id: "42", statusCode: http.StatusUnauthorized},
		{name: "Err404", id: "42", auth: true, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", id: "42", auth: true, err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.auth && tt.id == "42" {
				suite.uc.On("MarkRead", mock.Anything, testUserID, 42).Once().Return(tt.err)
			}

			w := suite.do(http.MethodPatch, "/notifications/"+tt.id+"/read", nil, tt.auth)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *NotificationsSuite) TestMarkAllRead() {
	tests := []struct {
		name       string
		auth       bool
		err        error
		statusCode int
	}{
		{name: "Ok200", auth: true, statusCode: http.StatusOK},
		{name: "Err401", statusCode: http.StatusUnauthorized},
		{name: "Err500", auth: true, err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.auth {
				suite.uc.On("MarkAllRead", mock.Anything, testUserID).Once().Return(int64(2), tt.err)
			}

			w := suite.do(http.MethodPatch, "/notifications/read-all", nil, tt.auth)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				suite.Contains(w.Body.String(), `"updated":2`)
			}
		})
	}
}

func (suite *NotificationsSuite) TestRegisterDevice() {
	tests := []struct {
		name       string
		body       string
		auth       bool
		wantCall   bool
		err        error
		statusCode int
	}{
		{name: "Ok200", body: `{"platform":"android","token":"tok"}`, auth: true, wantCall: true, statusCode: http.StatusOK},
		{name: "Err400BadPlatform", body: `{"platform":"windows","token":"tok"}`, auth: true, statusCode: http.StatusBadRequest},
		{name: "Err400NoToken", body: `{"platform":"ios"}`, auth: true, statusCode: http.StatusBadRequest},
		{name: "Err400NotJSON", body: `{`, auth: true, statusCode: http.StatusBadRequest},
		{name: "Err401", body: `{"platform":"web","token":"tok"}`, statusCode: http.StatusUnauthorized},
		{name: "Err500", body: `{"platform":"web","token":"tok"}`, auth: true, wantCall: true, err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantCall {
				suite.uc.On("RegisterDevice", mock.Anything, mock.MatchedBy(func(d models.UserDevice) bool {
					return d.UserID == testUserID && d.Token == "tok"
				})).Once().Return(models.UserDevice{ID: 1, UserID: testUserID, Token: "tok"}, tt.err)
			}

			w := suite.do(http.MethodPost, "/users/me/devices", []byte(tt.body), tt.auth)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *NotificationsSuite) TestDeleteDevice() {
	tests := []struct {
		name       string
		auth       bool
		err        error
		statusCode int
	}{
		{name: "Ok200", auth: true, statusCode: http.StatusOK},
		{name: "Err401", statusCode: http.StatusUnauthorized},
		{name: "Err404", auth: true, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", auth: true, err: errInternal, statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.auth {
				suite.uc.On("DeleteDevice", mock.Anything, testUserID, "tok").Once().Return(tt.err)
			}

			w := suite.do(http.MethodDelete, "/users/me/devices/tok", nil, tt.auth)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}
