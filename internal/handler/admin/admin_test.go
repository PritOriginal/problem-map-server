package adminrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adminrest "github.com/PritOriginal/problem-map-server/internal/handler/admin"
	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	"github.com/PritOriginal/problem-map-server/internal/middleware/lang"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/guregu/null/v6"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type AdminSuite struct {
	suite.Suite
	r         *gin.Engine
	settings  *adminrest.MockSettings
	markTypes *adminrest.MockMarkTypes
}

func TestAdmin(t *testing.T) {
	suite.Run(t, new(AdminSuite))
}

func (suite *AdminSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte("1234")})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.settings = adminrest.NewMockSettings(suite.T())
	suite.markTypes = adminrest.NewMockMarkTypes(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	suite.r.Use(lang.New())
	adminrest.Register(suite.r, slogdiscard.NewDiscardLogger(), adminrest.Params{
		AuthMiddleware: authMiddleware,
		Settings:       suite.settings,
		MarkTypes:      suite.markTypes,
	})
}

func (suite *AdminSuite) do(method, path string, body string, role models.Role, headers ...string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		accessToken, err := token.CreateToken(time.Minute, 7, string(role), "1234")
		suite.Require().NoError(err)
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, req)
	return w
}

func (suite *AdminSuite) TestRequiresAdmin() {
	routes := []struct{ method, path string }{
		{"GET", "/admin/settings"}, {"PUT", "/admin/settings"}, {"GET", "/admin/settings/history"},
		{"GET", "/admin/mark-types"}, {"POST", "/admin/mark-types"}, {"PATCH", "/admin/mark-types/1"},
	}
	for _, rt := range routes {
		for _, tt := range []struct {
			role       models.Role
			statusCode int
		}{
			{role: "", statusCode: http.StatusUnauthorized},
			{role: models.RoleUser, statusCode: http.StatusForbidden},
			{role: models.RoleModerator, statusCode: http.StatusForbidden},
			{role: models.RoleService, statusCode: http.StatusForbidden},
		} {
			suite.Run(rt.method+" "+rt.path+" "+string(tt.role), func() {
				w := suite.do(rt.method, rt.path, "", tt.role)
				handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			})
		}
	}
}

func (suite *AdminSuite) TestGetSettings() {
	tests := []struct {
		name       string
		err        error
		statusCode int
	}{
		{name: "Ok200", statusCode: http.StatusOK},
		{name: "Err500", err: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			want := usecase.DefaultRuntimeSettings()
			suite.settings.On("Load", mock.Anything).Once().Return(want, tt.err)

			w := suite.do("GET", "/admin/settings", "", models.RoleAdmin)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[adminrest.SettingsResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal(want, resp.Payload.Settings)
			}
		})
	}
}

func (suite *AdminSuite) TestUpdateSettings() {
	valid := usecase.DefaultRuntimeSettings()
	valid.VoteThreshold = 4
	raw, err := json.Marshal(adminrest.UpdateSettingsRequest{Settings: &valid})
	suite.Require().NoError(err)

	tests := []struct {
		name       string
		body       string
		err        error
		statusCode int
	}{
		{name: "Ok200", body: string(raw), statusCode: http.StatusOK},
		{name: "Err400Malformed", body: `{"settings":`, statusCode: http.StatusBadRequest},
		{name: "Err400MissingSettings", body: `{}`, statusCode: http.StatusBadRequest},
		{name: "Err400OutOfRange", body: string(raw), err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err500", body: string(raw), err: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.name != "Err400Malformed" && tt.name != "Err400MissingSettings" {
				// The admin from the token (user 7) is recorded as the author.
				suite.settings.On("Update", mock.Anything, valid, 7).Once().Return(valid, tt.err)
			}

			w := suite.do("PUT", "/admin/settings", tt.body, models.RoleAdmin)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[adminrest.SettingsResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal(valid, resp.Payload.Settings)
			}
		})
	}
}

func (suite *AdminSuite) TestGetSettingsHistory() {
	oldValue := json.RawMessage(`{"vote_threshold":3}`)
	changes := []models.SettingChange{{
		ID: 1, Key: usecase.RuntimeSettingsKey,
		Old: &oldValue, New: json.RawMessage(`{"vote_threshold":4}`),
		UpdatedBy: null.IntFrom(7), UpdatedAt: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
	}}
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		err        error
		statusCode int
	}{
		{name: "Ok200Default", wantLimit: 20, statusCode: http.StatusOK},
		{name: "Ok200Limit", query: "?limit=5", wantLimit: 5, statusCode: http.StatusOK},
		{name: "Err400LimitTooBig", query: "?limit=101", statusCode: http.StatusBadRequest},
		{name: "Err400LimitZero", query: "?limit=0", statusCode: http.StatusBadRequest},
		{name: "Err500", wantLimit: 20, err: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantLimit > 0 {
				suite.settings.On("History", mock.Anything, tt.wantLimit).Once().Return(changes, tt.err)
			}

			w := suite.do("GET", "/admin/settings/history"+tt.query, "", models.RoleAdmin)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[adminrest.SettingsHistoryResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Require().Len(resp.Payload.Changes, 1)
				suite.Equal(int64(1), resp.Payload.Changes[0].ID)
				suite.JSONEq(`{"vote_threshold":4}`, string(resp.Payload.Changes[0].New))
			}
		})
	}
}

func (suite *AdminSuite) TestGetMarkTypes() {
	// LegacyID is filled by MarshalJSON, so the decoded copy carries it.
	types := []models.MarkType{{ID: 1, LegacyID: 1, Code: "garbage", Name: "Garbage", SLAHours: 72, Active: false, SortOrder: 1}}
	tests := []struct {
		name           string
		acceptLanguage string
		wantLang       models.Lang
		err            error
		statusCode     int
	}{
		{name: "Ok200RU", wantLang: models.LangRU, statusCode: http.StatusOK},
		{name: "Ok200EN", acceptLanguage: "en", wantLang: models.LangEN, statusCode: http.StatusOK},
		{name: "Err500", wantLang: models.LangRU, err: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.markTypes.On("List", mock.Anything, tt.wantLang).Once().Return(types, tt.err)

			w := suite.do("GET", "/admin/mark-types", "", models.RoleAdmin, "Accept-Language", tt.acceptLanguage)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[adminrest.MarkTypesResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal(types, resp.Payload.MarkTypes)
			}
		})
	}
}

func (suite *AdminSuite) TestCreateMarkType() {
	want := models.MarkTypeCreate{Code: "potholes", NameRU: "Ямы", NameEN: "Potholes", Icon: null.StringFrom("pit"), Color: null.StringFrom("#ff8800"), SLAHours: 48}
	body := `{"code":"potholes","name_ru":"Ямы","name_en":"Potholes","icon":"pit","color":"#ff8800","sla_hours":48}`
	tests := []struct {
		name       string
		body       string
		input      *models.MarkTypeCreate
		err        error
		statusCode int
	}{
		{name: "Ok201", body: body, input: &want, statusCode: http.StatusCreated},
		{name: "Ok201Minimal", body: `{"code":"potholes","name_ru":"Ямы","sla_hours":48}`, input: &models.MarkTypeCreate{Code: "potholes", NameRU: "Ямы", SLAHours: 48}, statusCode: http.StatusCreated},
		{name: "Err400NoCode", body: `{"name_ru":"Ямы","sla_hours":48}`, statusCode: http.StatusBadRequest},
		{name: "Err400NoSLA", body: `{"code":"potholes","name_ru":"Ямы"}`, statusCode: http.StatusBadRequest},
		{name: "Err400Invalid", body: body, input: &want, err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err409", body: body, input: &want, err: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err500", body: body, input: &want, err: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.input != nil {
				suite.markTypes.On("Create", mock.Anything, *tt.input, models.LangRU).Once().
					Return(models.MarkType{ID: 9, Code: tt.input.Code, Name: tt.input.NameRU, Active: true}, tt.err)
			}

			w := suite.do("POST", "/admin/mark-types", tt.body, models.RoleAdmin)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusCreated {
				var resp responses.Response[adminrest.MarkTypeResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal(9, resp.Payload.MarkType.ID)
			}
		})
	}
}

func (suite *AdminSuite) TestUpdateMarkType() {
	active := false
	order := 3
	empty := ""
	tests := []struct {
		name       string
		path       string
		body       string
		upd        *models.MarkTypeUpdate
		err        error
		statusCode int
	}{
		{name: "Ok200", path: "/admin/mark-types/2", body: `{"active":false,"sort_order":3}`, upd: &models.MarkTypeUpdate{Active: &active, SortOrder: &order}, statusCode: http.StatusOK},
		{name: "Ok200ClearIcon", path: "/admin/mark-types/2", body: `{"icon":""}`, upd: &models.MarkTypeUpdate{Icon: &empty}, statusCode: http.StatusOK},
		{name: "Err400BadId", path: "/admin/mark-types/abc", body: `{"active":false}`, statusCode: http.StatusBadRequest},
		{name: "Err400Malformed", path: "/admin/mark-types/2", body: `{"active":`, statusCode: http.StatusBadRequest},
		{name: "Err400Empty", path: "/admin/mark-types/2", body: `{}`, upd: &models.MarkTypeUpdate{}, err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err404", path: "/admin/mark-types/2", body: `{"active":false}`, upd: &models.MarkTypeUpdate{Active: &active}, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409", path: "/admin/mark-types/2", body: `{"active":false}`, upd: &models.MarkTypeUpdate{Active: &active}, err: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err500", path: "/admin/mark-types/2", body: `{"active":false}`, upd: &models.MarkTypeUpdate{Active: &active}, err: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.upd != nil {
				suite.markTypes.On("Update", mock.Anything, 2, *tt.upd, models.LangRU).Once().
					Return(models.MarkType{ID: 2, Active: active}, tt.err)
			}

			w := suite.do("PATCH", tt.path, tt.body, models.RoleAdmin)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				var resp responses.Response[adminrest.MarkTypeResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal(2, resp.Payload.MarkType.ID)
			}
		})
	}
}
