package webhooksrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	webhooksrest "github.com/PritOriginal/problem-map-server/internal/handler/webhooks"
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

type WebhooksSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *webhooksrest.MockWebhooks
}

func (suite *WebhooksSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(testKey)})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.uc = webhooksrest.NewMockWebhooks(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	webhooksrest.Register(suite.r, slogdiscard.NewDiscardLogger(), authMiddleware, suite.uc)
}

func TestWebhooks(t *testing.T) {
	suite.Run(t, new(WebhooksSuite))
}

// do performs a request as role (empty role means anonymous).
func (suite *WebhooksSuite) do(method, path string, body []byte, role models.Role) *httptest.ResponseRecorder {
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

func moderator() models.Actor { return models.Actor{UserID: testUserID, Role: models.RoleModerator} }

func webhook() models.Webhook {
	return models.Webhook{ID: 3, OwnerUserID: testUserID, URL: "https://example.org/hook", Secret: "s3cr3t", Events: []string{"mark.*"}, Active: true}
}

func (suite *WebhooksSuite) TestRoles() {
	tests := []struct {
		name       string
		role       models.Role
		statusCode int
	}{
		{name: "Anonymous401", statusCode: http.StatusUnauthorized},
		{name: "User403", role: models.RoleUser, statusCode: http.StatusForbidden},
		{name: "Moderator200", role: models.RoleModerator, statusCode: http.StatusOK},
		{name: "Admin200", role: models.RoleAdmin, statusCode: http.StatusOK},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode == http.StatusOK {
				suite.uc.On("List", mock.Anything, models.Actor{UserID: testUserID, Role: tt.role}).Once().Return([]models.Webhook{webhook()}, nil)
			}
			w := suite.do(http.MethodGet, "/webhooks", nil, tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *WebhooksSuite) TestCreateWebhook() {
	tests := []struct {
		name       string
		body       string
		wantModel  *models.Webhook
		err        error
		statusCode int
	}{
		{
			name:       "Created201",
			body:       `{"url":"https://example.org/hook","events":["mark.*","check.added"]}`,
			wantModel:  &models.Webhook{URL: "https://example.org/hook", Events: []string{"mark.*", "check.added"}},
			statusCode: http.StatusCreated,
		},
		{
			name:       "Created201OwnSecret",
			body:       `{"url":"https://example.org/hook","events":["*"],"secret":"my-own-secret-123"}`,
			wantModel:  &models.Webhook{URL: "https://example.org/hook", Events: []string{"*"}, Secret: "my-own-secret-123"},
			statusCode: http.StatusCreated,
		},
		{name: "Err400NoURL", body: `{"events":["*"]}`, statusCode: http.StatusBadRequest},
		{name: "Err400NoEvents", body: `{"url":"https://example.org/hook","events":[]}`, statusCode: http.StatusBadRequest},
		{name: "Err400ShortSecret", body: `{"url":"https://example.org/hook","events":["*"],"secret":"short"}`, statusCode: http.StatusBadRequest},
		{name: "Err400NotJSON", body: `{`, statusCode: http.StatusBadRequest},
		{
			name:       "Err400PrivateURL",
			body:       `{"url":"https://10.0.0.1/hook","events":["*"]}`,
			wantModel:  &models.Webhook{URL: "https://10.0.0.1/hook", Events: []string{"*"}},
			err:        usecase.ErrInvalidArgument,
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "Err500",
			body:       `{"url":"https://example.org/hook","events":["*"]}`,
			wantModel:  &models.Webhook{URL: "https://example.org/hook", Events: []string{"*"}},
			err:        errInternal,
			statusCode: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantModel != nil {
				created := webhook()
				created.Secret = "generated-or-own"
				suite.uc.On("Create", mock.Anything, moderator(), *tt.wantModel).Once().Return(created, tt.err)
			}

			w := suite.do(http.MethodPost, "/webhooks", []byte(tt.body), models.RoleModerator)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusCreated {
				return
			}

			var resp responses.Response[webhooksrest.CreateWebhookResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Equal("generated-or-own", resp.Payload.Secret, "the secret is returned once, next to the webhook")
			suite.Equal(3, resp.Payload.Webhook.ID)
			suite.NotContains(w.Body.String(), `"secret":"generated-or-own","webhook"`, "the webhook object itself never serialises the secret")
		})
	}
}

func (suite *WebhooksSuite) TestWebhookSecretNotSerialised() {
	suite.uc.On("List", mock.Anything, moderator()).Once().Return([]models.Webhook{webhook()}, nil)

	w := suite.do(http.MethodGet, "/webhooks", nil, models.RoleModerator)
	handlertest.AssertResponse(suite.T(), w, http.StatusOK)
	suite.NotContains(w.Body.String(), "s3cr3t")
	suite.Contains(w.Body.String(), `"events":["mark.*"]`)
}

func (suite *WebhooksSuite) TestUpdateWebhook() {
	active := false
	tests := []struct {
		name       string
		path       string
		body       string
		wantUpd    *models.WebhookUpdate
		err        error
		statusCode int
	}{
		{name: "Ok200Active", path: "/webhooks/3", body: `{"active":false}`, wantUpd: &models.WebhookUpdate{Active: &active}, statusCode: http.StatusOK},
		{name: "Ok200Events", path: "/webhooks/3", body: `{"events":["task.assigned"]}`, wantUpd: &models.WebhookUpdate{Events: []string{"task.assigned"}}, statusCode: http.StatusOK},
		{name: "Err400BadID", path: "/webhooks/abc", body: `{"active":false}`, statusCode: http.StatusBadRequest},
		{name: "Err400EmptyEvents", path: "/webhooks/3", body: `{"events":[]}`, statusCode: http.StatusBadRequest},
		{name: "Err400Empty", path: "/webhooks/3", body: `{}`, wantUpd: &models.WebhookUpdate{}, err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err403", path: "/webhooks/3", body: `{"active":false}`, wantUpd: &models.WebhookUpdate{Active: &active}, err: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err404", path: "/webhooks/3", body: `{"active":false}`, wantUpd: &models.WebhookUpdate{Active: &active}, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantUpd != nil {
				suite.uc.On("Update", mock.Anything, moderator(), 3, *tt.wantUpd).Once().Return(webhook(), tt.err)
			}
			w := suite.do(http.MethodPatch, tt.path, []byte(tt.body), models.RoleModerator)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *WebhooksSuite) TestDeleteWebhook() {
	tests := []struct {
		name       string
		path       string
		call       bool
		err        error
		statusCode int
	}{
		{name: "Ok200", path: "/webhooks/3", call: true, statusCode: http.StatusOK},
		{name: "Err400", path: "/webhooks/x", statusCode: http.StatusBadRequest},
		{name: "Err403", path: "/webhooks/3", call: true, err: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err404", path: "/webhooks/3", call: true, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.call {
				suite.uc.On("Delete", mock.Anything, moderator(), 3).Once().Return(tt.err)
			}
			w := suite.do(http.MethodDelete, tt.path, nil, models.RoleModerator)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				suite.JSONEq(`{"success":true,"payload":{"webhook_id":3}}`, w.Body.String())
			}
		})
	}
}

func (suite *WebhooksSuite) TestGetDeliveries() {
	tests := []struct {
		name       string
		query      string
		wantP      *models.Pagination
		err        error
		statusCode int
	}{
		{name: "Ok200", wantP: &models.Pagination{Limit: 100}, statusCode: http.StatusOK},
		{name: "Ok200Paged", query: "?limit=10&offset=20", wantP: &models.Pagination{Limit: 10, Offset: 20}, statusCode: http.StatusOK},
		{name: "Err400Limit", query: "?limit=0", statusCode: http.StatusBadRequest},
		{name: "Err403", wantP: &models.Pagination{Limit: 100}, err: usecase.ErrForbidden, statusCode: http.StatusForbidden},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantP != nil {
				page := models.Page[models.WebhookDelivery]{Items: []models.WebhookDelivery{{ID: 1, WebhookID: 3, Attempt: 2, StatusCode: null.IntFrom(500), Error: null.StringFrom("unexpected status 500"), NextAttemptAt: null.TimeFrom(time.Now())}}, Total: 1}
				suite.uc.On("ListDeliveries", mock.Anything, moderator(), 3, *tt.wantP).Once().Return(page, tt.err)
			}
			w := suite.do(http.MethodGet, "/webhooks/3/deliveries"+tt.query, nil, models.RoleModerator)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[webhooksrest.GetDeliveriesResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal(tt.wantP.Limit, resp.Meta.Limit)
				suite.Equal(1, resp.Meta.Total)
				suite.Require().Len(resp.Payload.Deliveries, 1)
				suite.Equal(int64(500), resp.Payload.Deliveries[0].StatusCode.ValueOrZero())
			}
		})
	}
}

func (suite *WebhooksSuite) TestTestWebhook() {
	tests := []struct {
		name       string
		err        error
		statusCode int
	}{
		{name: "Ok200", statusCode: http.StatusOK},
		{name: "Err404", err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", err: errInternal, statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			d := models.WebhookDelivery{ID: 9, WebhookID: 3, Subject: models.WebhookSubjectTest, Attempt: 1, StatusCode: null.IntFrom(200), DeliveredAt: null.TimeFrom(time.Now())}
			suite.uc.On("SendTest", mock.Anything, moderator(), 3).Once().Return(d, tt.err)

			w := suite.do(http.MethodPost, "/webhooks/3/test", nil, models.RoleModerator)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				suite.Contains(w.Body.String(), `"subject":"webhook.test"`)
			}
		})
	}
}
