package usersrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
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

func (suite *UsersSuite) SetupTest() {
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
			errGetUserById: usecase.ErrNotFound,
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

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
			errGetUserById: usecase.ErrNotFound,
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

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

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
			suite.uc.On("ListUsers", mock.Anything, models.Pagination{Limit: models.DefaultLimit}).Once().
				Return(models.Page[models.User]{Items: []models.User{testUser()}, Total: 1}, tt.errGetUsers)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users", nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == 200 {
				var resp responses.Response[map[string][]map[string]any]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Len(resp.Payload["users"], 1)
				suite.NotContains(resp.Payload["users"][0], "login")
				suite.NotContains(resp.Payload["users"][0], "home_point")
				suite.Equal(&responses.ListMeta{Limit: models.DefaultLimit, Offset: 0, Total: 1}, resp.Meta)
			}
		})
	}
}

func (suite *UsersSuite) TestGetUsersPagination() {
	tests := []struct {
		name       string
		query      string
		pagination models.Pagination
		statusCode int
	}{
		{
			name:       "Ok",
			query:      "?limit=20&offset=40",
			pagination: models.Pagination{Limit: 20, Offset: 40},
			statusCode: 200,
		},
		{
			name:       "OkMaxLimit",
			query:      "?limit=500",
			pagination: models.Pagination{Limit: 500},
			statusCode: 200,
		},
		{
			name:       "ErrLimitTooBig",
			query:      "?limit=501",
			statusCode: 400,
		},
		{
			name:       "ErrLimitZero",
			query:      "?limit=0",
			statusCode: 400,
		},
		{
			name:       "ErrNegativeOffset",
			query:      "?offset=-1",
			statusCode: 400,
		},
		{
			name:       "ErrNotANumber",
			query:      "?limit=abc",
			statusCode: 400,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode == 200 {
				suite.uc.On("ListUsers", mock.Anything, tt.pagination).Once().
					Return(models.Page[models.User]{Items: []models.User{}, Total: 0}, nil)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}

// bearer returns an Authorization header value for the given user and role.
func (suite *UsersSuite) bearer(userId int, role models.Role) string {
	accessToken, err := token.CreateToken(1*time.Minute, userId, string(role), "1234")
	suite.Require().NoError(err)
	return "Bearer " + accessToken
}

func (suite *UsersSuite) TestChangePassword() {
	tests := []struct {
		name       string
		noToken    bool
		rawReq     string
		req        usersrest.ChangePasswordRequest
		wantUCCall bool
		errChange  error
		statusCode int
	}{
		{name: "Ok204", req: usersrest.ChangePasswordRequest{OldPassword: "oldpassword", NewPassword: "newpassword"}, wantUCCall: true, statusCode: http.StatusNoContent},
		{name: "Err401NoToken", noToken: true, req: usersrest.ChangePasswordRequest{OldPassword: "oldpassword", NewPassword: "newpassword"}, statusCode: http.StatusUnauthorized},
		{name: "Err400InvalidJSON", rawReq: "{", statusCode: http.StatusBadRequest},
		{name: "Err400ShortNew", req: usersrest.ChangePasswordRequest{OldPassword: "oldpassword", NewPassword: "short"}, statusCode: http.StatusBadRequest},
		{name: "Err400MissingOld", req: usersrest.ChangePasswordRequest{NewPassword: "newpassword"}, statusCode: http.StatusBadRequest},
		{name: "Err403WrongOld", req: usersrest.ChangePasswordRequest{OldPassword: "oldpassword", NewPassword: "newpassword"}, wantUCCall: true, errChange: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err404", req: usersrest.ChangePasswordRequest{OldPassword: "oldpassword", NewPassword: "newpassword"}, wantUCCall: true, errChange: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", req: usersrest.ChangePasswordRequest{OldPassword: "oldpassword", NewPassword: "newpassword"}, wantUCCall: true, errChange: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantUCCall {
				suite.uc.On("ChangePassword", mock.Anything, 1, tt.req.OldPassword, tt.req.NewPassword).Once().Return(tt.errChange)
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
			req := httptest.NewRequest(http.MethodPost, "/users/me/password", buf)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(1, models.RoleUser))
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func testStats() models.UserStats {
	return models.UserStats{Rating: 7, MarksTotal: 3, MarksConfirmed: 2, MarksRefuted: 1, ChecksTotal: 5, ChecksCorrect: 4, TasksCompleted: 2}
}

func (suite *UsersSuite) TestGetUserStats() {
	tests := []struct {
		name       string
		id         string
		errStats   error
		statusCode int
	}{
		{name: "Ok200", id: "1", statusCode: 200},
		{name: "Err400", id: "a", statusCode: 400},
		{name: "Err404", id: "1", errStats: usecase.ErrNotFound, statusCode: 404},
		{name: "Err500", id: "1", errStats: errors.New(""), statusCode: 500},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode != 400 {
				suite.uc.On("GetUserStats", mock.Anything, 1).Once().Return(testStats(), tt.errStats)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users/"+tt.id+"/stats", nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == 200 {
				var resp responses.Response[usersrest.GetUserStatsResponse]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal(testStats(), resp.Payload.Stats)
			}
		})
	}
}

func (suite *UsersSuite) TestGetMyStats() {
	tests := []struct {
		name       string
		noToken    bool
		errStats   error
		statusCode int
	}{
		{name: "Ok200", statusCode: 200},
		{name: "Err401", noToken: true, statusCode: 401},
		{name: "Err500", errStats: errors.New(""), statusCode: 500},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.noToken {
				suite.uc.On("GetUserStats", mock.Anything, 1).Once().Return(testStats(), tt.errStats)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users/me/stats", nil)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(1, models.RoleUser))
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *UsersSuite) TestSetRole() {
	tests := []struct {
		name       string
		noToken    bool
		role       models.Role
		id         string
		rawReq     string
		req        usersrest.SetRoleRequest
		wantUCCall bool
		errSetRole error
		statusCode int
	}{
		{name: "Ok204", role: models.RoleAdmin, id: "2", req: usersrest.SetRoleRequest{Role: models.RoleModerator}, wantUCCall: true, statusCode: http.StatusNoContent},
		{name: "Err401NoToken", noToken: true, id: "2", req: usersrest.SetRoleRequest{Role: models.RoleModerator}, statusCode: http.StatusUnauthorized},
		{name: "Err403User", role: models.RoleUser, id: "2", req: usersrest.SetRoleRequest{Role: models.RoleModerator}, statusCode: http.StatusForbidden},
		{name: "Err403Moderator", role: models.RoleModerator, id: "2", req: usersrest.SetRoleRequest{Role: models.RoleModerator}, statusCode: http.StatusForbidden},
		{name: "Err400BadId", role: models.RoleAdmin, id: "abc", req: usersrest.SetRoleRequest{Role: models.RoleModerator}, statusCode: http.StatusBadRequest},
		{name: "Err400InvalidJSON", role: models.RoleAdmin, id: "2", rawReq: "{", statusCode: http.StatusBadRequest},
		{name: "Err400UnknownRole", role: models.RoleAdmin, id: "2", req: usersrest.SetRoleRequest{Role: "root"}, statusCode: http.StatusBadRequest},
		{name: "Err400EmptyRole", role: models.RoleAdmin, id: "2", req: usersrest.SetRoleRequest{}, statusCode: http.StatusBadRequest},
		{name: "Err404", role: models.RoleAdmin, id: "2", req: usersrest.SetRoleRequest{Role: models.RoleUser}, wantUCCall: true, errSetRole: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", role: models.RoleAdmin, id: "2", req: usersrest.SetRoleRequest{Role: models.RoleUser}, wantUCCall: true, errSetRole: errors.New(""), statusCode: http.StatusInternalServerError},
		{name: "Err403LastAdminSelfDemote", role: models.RoleAdmin, id: "1", req: usersrest.SetRoleRequest{Role: models.RoleUser}, wantUCCall: true, errSetRole: usecase.ErrForbidden, statusCode: http.StatusForbidden},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantUCCall {
				id, err := strconv.Atoi(tt.id)
				suite.Require().NoError(err)
				// The bearer token belongs to user 1.
				suite.uc.On("SetRole", mock.Anything, 1, id, tt.req.Role).Once().Return(tt.errSetRole)
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
			req := httptest.NewRequest(http.MethodPatch, "/users/"+tt.id+"/role", buf)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(1, tt.role))
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *UsersSuite) TestGetLeaderboard() {
	tests := []struct {
		name       string
		query      string
		pagination models.Pagination
		errList    error
		statusCode int
	}{
		{name: "Ok200", pagination: models.Pagination{Limit: models.DefaultLimit}, statusCode: 200},
		{name: "OkPaginated", query: "?limit=5&offset=10", pagination: models.Pagination{Limit: 5, Offset: 10}, statusCode: 200},
		{name: "Err400", query: "?limit=0", statusCode: 400},
		{name: "Err500", pagination: models.Pagination{Limit: models.DefaultLimit}, errList: errors.New(""), statusCode: 500},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode != 400 {
				suite.uc.On("ListLeaderboard", mock.Anything, tt.pagination).Once().
					Return(models.Page[models.User]{Items: []models.User{testUser()}, Total: 1}, tt.errList)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/leaderboard"+tt.query, nil)

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == 200 {
				var resp responses.Response[map[string][]map[string]any]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Require().Len(resp.Payload["leaderboard"], 1)
				entry := resp.Payload["leaderboard"][0]
				suite.Equal(float64(1), entry["user_id"])
				suite.Equal("name", entry["username"])
				suite.Equal(float64(3), entry["rating"])
				suite.NotContains(entry, "login")
				suite.NotContains(entry, "home_point")
				suite.NotContains(entry, "role")
				suite.Equal(&responses.ListMeta{Limit: tt.pagination.Limit, Offset: tt.pagination.Offset, Total: 1}, resp.Meta)
			}
		})
	}
}

func (suite *UsersSuite) TestGetRatingEvents() {
	tests := []struct {
		name       string
		id         string
		noToken    bool
		requester  usecase.Requester
		errList    error
		statusCode int
	}{
		{name: "Ok200Owner", id: "1", requester: usecase.Requester{ID: 1, Role: models.RoleUser}, statusCode: 200},
		{name: "Ok200Moderator", id: "1", requester: usecase.Requester{ID: 2, Role: models.RoleModerator}, statusCode: 200},
		{name: "Err400", id: "a", requester: usecase.Requester{ID: 1, Role: models.RoleUser}, statusCode: 400},
		{name: "Err401", id: "1", noToken: true, statusCode: 401},
		{name: "Err403", id: "1", requester: usecase.Requester{ID: 2, Role: models.RoleUser}, errList: usecase.ErrForbidden, statusCode: 403},
		{name: "Err500", id: "1", requester: usecase.Requester{ID: 1, Role: models.RoleUser}, errList: errors.New(""), statusCode: 500},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.noToken && tt.statusCode != 400 {
				suite.uc.On("ListRatingEvents", mock.Anything, tt.requester, 1, models.Pagination{Limit: models.DefaultLimit}).Once().
					Return(models.Page[models.RatingEvent]{Items: []models.RatingEvent{{ID: 1, UserID: 1, Delta: 2, Reason: models.RatingReasonCheckCorrect}}, Total: 1}, tt.errList)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users/"+tt.id+"/rating-events", nil)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(tt.requester.ID, tt.requester.Role))
			}

			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == 200 {
				var resp responses.Response[usersrest.GetRatingEventsResponse]
				suite.NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Require().Len(resp.Payload.Events, 1)
				suite.Equal(models.RatingReasonCheckCorrect, resp.Payload.Events[0].Reason)
				suite.Equal(&responses.ListMeta{Limit: models.DefaultLimit, Offset: 0, Total: 1}, resp.Meta)
			}
		})
	}
}
