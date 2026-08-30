package reportsrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	reportsrest "github.com/PritOriginal/problem-map-server/internal/handler/reports"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
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

var errInternal = errors.New("boom")

type ReportsSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *reportsrest.MockReports
}

func (suite *ReportsSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(testKey)})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.uc = reportsrest.NewMockReports(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	reportsrest.Register(suite.r, slogdiscard.NewDiscardLogger(), authMiddleware, suite.uc)
}

func TestReports(t *testing.T) {
	suite.Run(t, new(ReportsSuite))
}

// do performs a request as role (empty role means anonymous).
func (suite *ReportsSuite) do(method, path string, body []byte, role models.Role) *httptest.ResponseRecorder {
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

func report() models.Report {
	return models.Report{ID: 11, ReporterID: testUserID, TargetType: models.ReportTargetMark, TargetID: 5, Reason: models.ReportReasonSpam, Status: models.ReportStatusOpen}
}

func (suite *ReportsSuite) TestRoles() {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		role       models.Role
		expect     func()
		statusCode int
	}{
		{name: "CreateAnonymous401", method: http.MethodPost, path: "/reports", body: `{"target_type":"mark","target_id":5,"reason":"spam"}`, statusCode: http.StatusUnauthorized},
		{
			name: "CreateUser201", method: http.MethodPost, path: "/reports", body: `{"target_type":"mark","target_id":5,"reason":"spam"}`, role: models.RoleUser,
			expect: func() {
				suite.uc.On("Create", mock.Anything, mock.Anything).Once().Return(report(), nil)
			},
			statusCode: http.StatusCreated,
		},
		{name: "QueueAnonymous401", method: http.MethodGet, path: "/moderation/queue", statusCode: http.StatusUnauthorized},
		{name: "QueueUser403", method: http.MethodGet, path: "/moderation/queue", role: models.RoleUser, statusCode: http.StatusForbidden},
		{name: "QueueService403", method: http.MethodGet, path: "/moderation/queue", role: models.RoleService, statusCode: http.StatusForbidden},
		{
			name: "QueueModerator200", method: http.MethodGet, path: "/moderation/queue", role: models.RoleModerator,
			expect: func() {
				suite.uc.On("ListQueue", mock.Anything, mock.Anything).Once().Return(models.Page[models.ReportWithTarget]{Items: []models.ReportWithTarget{}}, nil)
			},
			statusCode: http.StatusOK,
		},
		{
			name: "QueueAdmin200", method: http.MethodGet, path: "/moderation/queue", role: models.RoleAdmin,
			expect: func() {
				suite.uc.On("ListQueue", mock.Anything, mock.Anything).Once().Return(models.Page[models.ReportWithTarget]{Items: []models.ReportWithTarget{}}, nil)
			},
			statusCode: http.StatusOK,
		},
		{name: "MineAnonymous401", method: http.MethodGet, path: "/moderation/reports/mine", statusCode: http.StatusUnauthorized},
		{
			name: "MineUser200", method: http.MethodGet, path: "/moderation/reports/mine", role: models.RoleUser,
			expect: func() {
				suite.uc.On("ListMine", mock.Anything, testUserID, models.Pagination{Limit: models.DefaultLimit}).Once().Return(models.Page[models.Report]{Items: []models.Report{}}, nil)
			},
			statusCode: http.StatusOK,
		},
		{name: "ResolveUser403", method: http.MethodPatch, path: "/moderation/reports/11", body: `{"status":"resolved"}`, role: models.RoleUser, statusCode: http.StatusForbidden},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.expect != nil {
				tt.expect()
			}
			var body []byte
			if tt.body != "" {
				body = []byte(tt.body)
			}
			w := suite.do(tt.method, tt.path, body, tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *ReportsSuite) TestCreateReport() {
	tests := []struct {
		name       string
		body       string
		wantModel  *models.Report
		err        error
		statusCode int
	}{
		{
			name:       "Created201",
			body:       `{"target_type":"mark","target_id":5,"reason":"spam","comment":"реклама"}`,
			wantModel:  &models.Report{ReporterID: testUserID, TargetType: models.ReportTargetMark, TargetID: 5, Reason: models.ReportReasonSpam, Comment: "реклама"},
			statusCode: http.StatusCreated,
		},
		{
			name:       "Created201Comment",
			body:       `{"target_type":"comment","target_id":42,"reason":"other"}`,
			wantModel:  &models.Report{ReporterID: testUserID, TargetType: models.ReportTargetComment, TargetID: 42, Reason: models.ReportReasonOther},
			statusCode: http.StatusCreated,
		},
		{name: "Err400UnknownTargetType", body: `{"target_type":"video","target_id":5,"reason":"spam"}`, statusCode: http.StatusBadRequest},
		{name: "Err400UnknownReason", body: `{"target_type":"mark","target_id":5,"reason":"boring"}`, statusCode: http.StatusBadRequest},
		{name: "Err400NoTargetID", body: `{"target_type":"mark","reason":"spam"}`, statusCode: http.StatusBadRequest},
		{name: "Err400ZeroTargetID", body: `{"target_type":"comment","target_id":0,"reason":"spam"}`, statusCode: http.StatusBadRequest},
		{name: "Err400LongComment", body: `{"target_type":"mark","target_id":5,"reason":"spam","comment":"` + strings.Repeat("ж", models.MaxReportCommentLen+1) + `"}`, statusCode: http.StatusBadRequest},
		{name: "Err400NotJSON", body: `{`, statusCode: http.StatusBadRequest},
		{
			name:       "Err403OwnMark",
			body:       `{"target_type":"mark","target_id":5,"reason":"spam"}`,
			wantModel:  &models.Report{ReporterID: testUserID, TargetType: models.ReportTargetMark, TargetID: 5, Reason: models.ReportReasonSpam},
			err:        usecase.ErrForbidden,
			statusCode: http.StatusForbidden,
		},
		{
			name:       "Err404",
			body:       `{"target_type":"check","target_id":9,"reason":"offensive"}`,
			wantModel:  &models.Report{ReporterID: testUserID, TargetType: models.ReportTargetCheck, TargetID: 9, Reason: models.ReportReasonOffensive},
			err:        usecase.ErrNotFound,
			statusCode: http.StatusNotFound,
		},
		{
			name:       "Err409Repeat",
			body:       `{"target_type":"mark","target_id":5,"reason":"spam"}`,
			wantModel:  &models.Report{ReporterID: testUserID, TargetType: models.ReportTargetMark, TargetID: 5, Reason: models.ReportReasonSpam},
			err:        usecase.ErrConflict,
			statusCode: http.StatusConflict,
		},
		{
			name:       "Err429DailyLimit",
			body:       `{"target_type":"mark","target_id":5,"reason":"spam"}`,
			wantModel:  &models.Report{ReporterID: testUserID, TargetType: models.ReportTargetMark, TargetID: 5, Reason: models.ReportReasonSpam},
			err:        usecase.ErrTooManyRequests,
			statusCode: http.StatusTooManyRequests,
		},
		{
			name:       "Err500",
			body:       `{"target_type":"mark","target_id":5,"reason":"spam"}`,
			wantModel:  &models.Report{ReporterID: testUserID, TargetType: models.ReportTargetMark, TargetID: 5, Reason: models.ReportReasonSpam},
			err:        errInternal,
			statusCode: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantModel != nil {
				suite.uc.On("Create", mock.Anything, *tt.wantModel).Once().Return(report(), tt.err)
			}

			w := suite.do(http.MethodPost, "/reports", []byte(tt.body), models.RoleUser)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusCreated {
				return
			}

			var resp responses.Response[reportsrest.CreateReportResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Equal(11, resp.Payload.Report.ID)
			suite.Equal(models.ReportStatusOpen, resp.Payload.Report.Status)
		})
	}
}

// TestCreateReportRequest_CommentLimit keeps the binding tag in sync with
// models.MaxReportCommentLen.
func (suite *ReportsSuite) TestCreateReportRequest_CommentLimit() {
	field, ok := reflect.TypeOf(reportsrest.CreateReportRequest{}).FieldByName("Comment")
	suite.Require().True(ok)
	suite.Equal("max="+strconv.Itoa(models.MaxReportCommentLen), field.Tag.Get("binding"))
}

func (suite *ReportsSuite) TestGetQueue() {
	item := models.ReportWithTarget{
		Report: report(),
		Target: models.ReportTarget{Type: models.ReportTargetMark, ID: 5, Mark: &models.MarkBrief{ID: 5, Description: "Свалка", Hidden: true}},
	}

	tests := []struct {
		name        string
		query       string
		wantFilters *models.GetReportsFilters
		err         error
		statusCode  int
	}{
		{
			name:        "Ok200DefaultsToOpen",
			wantFilters: &models.GetReportsFilters{Status: models.ReportStatusOpen, Pagination: models.Pagination{Limit: models.DefaultLimit}},
			statusCode:  http.StatusOK,
		},
		{
			name:        "Ok200Filters",
			query:       "?status=dismissed&target_type=check&limit=10&offset=20",
			wantFilters: &models.GetReportsFilters{Status: models.ReportStatusDismissed, TargetType: models.ReportTargetCheck, Pagination: models.Pagination{Limit: 10, Offset: 20}},
			statusCode:  http.StatusOK,
		},
		{name: "Err400Status", query: "?status=weird", statusCode: http.StatusBadRequest},
		{name: "Err400TargetType", query: "?target_type=video", statusCode: http.StatusBadRequest},
		{name: "Err400Limit", query: "?limit=0", statusCode: http.StatusBadRequest},
		{
			name:        "Err500",
			wantFilters: &models.GetReportsFilters{Status: models.ReportStatusOpen, Pagination: models.Pagination{Limit: models.DefaultLimit}},
			err:         errInternal,
			statusCode:  http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantFilters != nil {
				suite.uc.On("ListQueue", mock.Anything, *tt.wantFilters).Once().Return(models.Page[models.ReportWithTarget]{Items: []models.ReportWithTarget{item}, Total: 1}, tt.err)
			}

			w := suite.do(http.MethodGet, "/moderation/queue"+tt.query, nil, models.RoleModerator)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusOK {
				return
			}

			var resp responses.Response[reportsrest.GetQueueResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Require().Len(resp.Payload.Reports, 1)
			suite.Equal(11, resp.Payload.Reports[0].ID)
			suite.Require().NotNil(resp.Payload.Reports[0].Target.Mark)
			suite.True(resp.Payload.Reports[0].Target.Mark.Hidden)
			suite.Require().NotNil(resp.Meta)
			suite.Equal(1, resp.Meta.Total)
		})
	}
}

func (suite *ReportsSuite) TestGetMyReports() {
	tests := []struct {
		name       string
		query      string
		wantPage   *models.Pagination
		err        error
		statusCode int
	}{
		{name: "Ok200", query: "?limit=5", wantPage: &models.Pagination{Limit: 5}, statusCode: http.StatusOK},
		{name: "Err400Limit", query: "?limit=abc", statusCode: http.StatusBadRequest},
		{name: "Err500", wantPage: &models.Pagination{Limit: models.DefaultLimit}, err: errInternal, statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantPage != nil {
				suite.uc.On("ListMine", mock.Anything, testUserID, *tt.wantPage).Once().Return(models.Page[models.Report]{Items: []models.Report{report()}, Total: 1}, tt.err)
			}

			w := suite.do(http.MethodGet, "/moderation/reports/mine"+tt.query, nil, models.RoleUser)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusOK {
				return
			}

			var resp responses.Response[reportsrest.GetMyReportsResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Len(resp.Payload.Reports, 1)
		})
	}
}

func (suite *ReportsSuite) TestResolveReport() {
	decided := report()
	decided.Status = models.ReportStatusResolved

	tests := []struct {
		name       string
		id         string
		body       string
		wantStatus models.ReportStatus
		err        error
		statusCode int
	}{
		{name: "Ok200Resolved", id: "11", body: `{"status":"resolved"}`, wantStatus: models.ReportStatusResolved, statusCode: http.StatusOK},
		{name: "Ok200Dismissed", id: "11", body: `{"status":"dismissed"}`, wantStatus: models.ReportStatusDismissed, statusCode: http.StatusOK},
		{name: "Err400Open", id: "11", body: `{"status":"open"}`, statusCode: http.StatusBadRequest},
		{name: "Err400NoStatus", id: "11", body: `{}`, statusCode: http.StatusBadRequest},
		{name: "Err400Id", id: "abc", body: `{"status":"resolved"}`, statusCode: http.StatusBadRequest},
		{name: "Err404", id: "11", body: `{"status":"resolved"}`, wantStatus: models.ReportStatusResolved, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409Decided", id: "11", body: `{"status":"resolved"}`, wantStatus: models.ReportStatusResolved, err: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err500", id: "11", body: `{"status":"resolved"}`, wantStatus: models.ReportStatusResolved, err: errInternal, statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantStatus != "" {
				suite.uc.On("Resolve", mock.Anything, moderator(), 11, tt.wantStatus).Once().Return(decided, tt.err)
			}

			w := suite.do(http.MethodPatch, "/moderation/reports/"+tt.id, []byte(tt.body), models.RoleModerator)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusOK {
				return
			}

			var resp responses.Response[reportsrest.ResolveReportResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Equal(models.ReportStatusResolved, resp.Payload.Report.Status)
		})
	}
}
