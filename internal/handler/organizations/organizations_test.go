package organizationsrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	organizationsrest "github.com/PritOriginal/problem-map-server/internal/handler/organizations"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/brianvoe/gofakeit/v7"
	"github.com/gin-gonic/gin"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type OrganizationsSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *organizationsrest.MockOrganizations
}

func (suite *OrganizationsSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte("1234")})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.uc = organizationsrest.NewMockOrganizations(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	organizationsrest.Register(suite.r, slogdiscard.NewDiscardLogger(), authMiddleware, suite.uc)
}

func TestOrganizations(t *testing.T) {
	suite.Run(t, new(OrganizationsSuite))
}

// bearer returns an Authorization header value for user id 1 with the role.
func (suite *OrganizationsSuite) bearer(role models.Role) string {
	accessToken, err := token.CreateToken(time.Minute, 1, string(role), "1234")
	suite.Require().NoError(err)
	return "Bearer " + accessToken
}

func (suite *OrganizationsSuite) do(method, path string, body []byte, contentType string, role models.Role) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if role != "" {
		req.Header.Set("Authorization", suite.bearer(role))
	}
	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, req)
	return w
}

func testOrg() models.Organization {
	return models.Organization{ID: 3, Name: "Водоканал", Description: "трубы", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (suite *OrganizationsSuite) TestList() {
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
			suite.uc.On("List", mock.Anything).Once().Return([]models.OrganizationRef{{ID: 3, Name: "Водоканал"}}, tt.err)

			w := suite.do("GET", "/organizations", nil, "", "")
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == http.StatusOK {
				var resp responses.Response[organizationsrest.ListOrganizationsResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Equal([]models.OrganizationRef{{ID: 3, Name: "Водоканал"}}, resp.Payload.Organizations)
			}
		})
	}
}

func (suite *OrganizationsSuite) TestCreate() {
	tests := []struct {
		name       string
		role       models.Role
		body       string
		err        error
		statusCode int
	}{
		{name: "Ok201", role: models.RoleAdmin, body: `{"name":"Водоканал","description":"трубы"}`, statusCode: http.StatusCreated},
		{name: "Err400", role: models.RoleAdmin, body: `{"description":"без имени"}`, statusCode: http.StatusBadRequest},
		{name: "Err401", role: "", body: `{"name":"x"}`, statusCode: http.StatusUnauthorized},
		{name: "Err403Moderator", role: models.RoleModerator, body: `{"name":"x"}`, statusCode: http.StatusForbidden},
		{name: "Err403Service", role: models.RoleService, body: `{"name":"x"}`, statusCode: http.StatusForbidden},
		{name: "Err500", role: models.RoleAdmin, body: `{"name":"x"}`, err: errors.New("db"), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.role == models.RoleAdmin && tt.statusCode != http.StatusBadRequest {
				suite.uc.On("Create", mock.Anything, mock.AnythingOfType("models.Organization")).Once().Return(testOrg(), tt.err)
			}

			w := suite.do("POST", "/organizations", []byte(tt.body), "application/json", tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *OrganizationsSuite) TestUpdate() {
	tests := []struct {
		name       string
		id         string
		body       string
		err        error
		statusCode int
	}{
		{name: "Ok200", id: "3", body: `{"name":"Новое"}`, statusCode: http.StatusOK},
		{name: "Err400Id", id: "x", body: `{"name":"Новое"}`, statusCode: http.StatusBadRequest},
		{name: "Err400Empty", id: "3", body: `{}`, err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err404", id: "3", body: `{"name":"Новое"}`, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.id == "3" {
				suite.uc.On("Update", mock.Anything, 3, mock.AnythingOfType("models.OrganizationUpdate")).Once().Return(testOrg(), tt.err)
			}

			w := suite.do("PATCH", "/organizations/"+tt.id, []byte(tt.body), "application/json", models.RoleAdmin)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *OrganizationsSuite) TestGetAndMine() {
	details := models.OrganizationDetails{Organization: testOrg(), Members: []models.User{}, Responsibilities: []models.OrganizationResponsibility{}}

	tests := []struct {
		name       string
		path       string
		role       models.Role
		method     string
		err        error
		statusCode int
	}{
		{name: "GetOk200", path: "/organizations/3", role: models.RoleAdmin, method: "Get", statusCode: http.StatusOK},
		{name: "GetErr404", path: "/organizations/3", role: models.RoleAdmin, method: "Get", err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "GetErr403Service", path: "/organizations/3", role: models.RoleService, statusCode: http.StatusForbidden},
		{name: "MineOk200Service", path: "/organizations/me", role: models.RoleService, method: "GetMine", statusCode: http.StatusOK},
		{name: "MineOk200Admin", path: "/organizations/me", role: models.RoleAdmin, method: "GetMine", statusCode: http.StatusOK},
		{name: "MineErr404", path: "/organizations/me", role: models.RoleService, method: "GetMine", err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "MineErr403User", path: "/organizations/me", role: models.RoleUser, statusCode: http.StatusForbidden},
		{name: "MineErr401", path: "/organizations/me", role: "", statusCode: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			switch tt.method {
			case "Get":
				suite.uc.On("Get", mock.Anything, 3).Once().Return(details, tt.err)
			case "GetMine":
				suite.uc.On("GetMine", mock.Anything, 1).Once().Return(details, tt.err)
			}

			w := suite.do("GET", tt.path, nil, "", tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *OrganizationsSuite) TestMembers() {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		call       string
		err        error
		statusCode int
	}{
		{name: "AddOk201", method: "POST", path: "/organizations/3/members", body: `{"user_id":7}`, call: "AddMember", statusCode: http.StatusCreated},
		{name: "AddErr400", method: "POST", path: "/organizations/3/members", body: `{"user_id":0}`, statusCode: http.StatusBadRequest},
		{name: "AddErr409", method: "POST", path: "/organizations/3/members", body: `{"user_id":7}`, call: "AddMember", err: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "AddErr404", method: "POST", path: "/organizations/3/members", body: `{"user_id":7}`, call: "AddMember", err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "RemoveOk200", method: "DELETE", path: "/organizations/3/members/7", call: "RemoveMember", statusCode: http.StatusOK},
		{name: "RemoveErr404", method: "DELETE", path: "/organizations/3/members/7", call: "RemoveMember", err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "RemoveErr400", method: "DELETE", path: "/organizations/3/members/x", statusCode: http.StatusBadRequest},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.call != "" {
				suite.uc.On(tt.call, mock.Anything, 3, 7).Once().Return(tt.err)
			}

			var body []byte
			if tt.body != "" {
				body = []byte(tt.body)
			}
			w := suite.do(tt.method, tt.path, body, "application/json", models.RoleAdmin)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *OrganizationsSuite) TestResponsibilities() {
	resp := models.OrganizationResponsibility{ID: 1, OrganizationID: 3, MarkTypeID: 2, BoundaryID: 5}

	tests := []struct {
		name       string
		method     string
		body       string
		err        error
		statusCode int
	}{
		{name: "AddOk201", method: "POST", body: `{"mark_type_id":2,"boundary_id":5}`, statusCode: http.StatusCreated},
		{name: "AddErr409", method: "POST", body: `{"mark_type_id":2,"boundary_id":5}`, err: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "AddErr400", method: "POST", body: `{"mark_type_id":2}`, statusCode: http.StatusBadRequest},
		{name: "AddErr400Ref", method: "POST", body: `{"mark_type_id":2,"boundary_id":5}`, err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "RemoveOk200", method: "DELETE", body: `{"mark_type_id":2,"boundary_id":5}`, statusCode: http.StatusOK},
		{name: "RemoveErr404", method: "DELETE", body: `{"mark_type_id":2,"boundary_id":5}`, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode != http.StatusBadRequest || tt.err != nil {
				if tt.method == "POST" {
					suite.uc.On("AddResponsibility", mock.Anything, withoutID(resp)).Once().Return(resp, tt.err)
				} else {
					suite.uc.On("RemoveResponsibility", mock.Anything, withoutID(resp)).Once().Return(tt.err)
				}
			}

			w := suite.do(tt.method, "/organizations/3/responsibilities", []byte(tt.body), "application/json", models.RoleAdmin)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *OrganizationsSuite) TestGetMarks() {
	page := models.Page[models.Mark]{Items: []models.Mark{{ID: 5, IsOverdue: true}}, Total: 1}

	tests := []struct {
		name       string
		role       models.Role
		query      string
		filters    models.GetOrganizationMarksFilters
		err        error
		statusCode int
	}{
		{
			name: "Ok200", role: models.RoleService, query: "?status_ids=2,7&overdue=true&limit=10&offset=5",
			filters:    models.GetOrganizationMarksFilters{MarkStatusIds: []int{2, 7}, Overdue: true, Pagination: models.Pagination{Limit: 10, Offset: 5}},
			statusCode: http.StatusOK,
		},
		{
			name: "Ok200Defaults", role: models.RoleAdmin,
			filters:    models.GetOrganizationMarksFilters{Pagination: models.Pagination{Limit: 100}},
			statusCode: http.StatusOK,
		},
		{name: "Err400StatusIds", role: models.RoleService, query: "?status_ids=a", statusCode: http.StatusBadRequest},
		{name: "Err400Limit", role: models.RoleService, query: "?limit=0", statusCode: http.StatusBadRequest},
		{
			name: "Err403NotMember", role: models.RoleService,
			filters: models.GetOrganizationMarksFilters{Pagination: models.Pagination{Limit: 100}},
			err:     usecase.ErrForbidden, statusCode: http.StatusForbidden,
		},
		{name: "Err403User", role: models.RoleUser, statusCode: http.StatusForbidden},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.statusCode == http.StatusOK || tt.err != nil {
				suite.uc.On("ListMarks", mock.Anything, models.Actor{UserID: 1, Role: tt.role}, 3, tt.filters).Once().Return(page, tt.err)
			}

			w := suite.do("GET", "/organizations/3/marks"+tt.query, nil, "", tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)

			if tt.statusCode == http.StatusOK {
				var resp responses.Response[organizationsrest.GetOrganizationMarksResponse]
				suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
				suite.Len(resp.Payload.Marks, 1)
				suite.True(resp.Payload.Marks[0].IsOverdue)
				suite.Equal(1, resp.Meta.Total)
			}
		})
	}
}

func (suite *OrganizationsSuite) TestStart() {
	tests := []struct {
		name       string
		role       models.Role
		id         string
		err        error
		statusCode int
	}{
		{name: "Ok200", role: models.RoleService, id: "5", statusCode: http.StatusOK},
		{name: "Err400", role: models.RoleService, id: "x", statusCode: http.StatusBadRequest},
		{name: "Err403User", role: models.RoleUser, id: "5", statusCode: http.StatusForbidden},
		{name: "Err403Admin", role: models.RoleAdmin, id: "5", statusCode: http.StatusForbidden},
		{name: "Err403NotMember", role: models.RoleService, id: "5", err: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err409", role: models.RoleService, id: "5", err: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err404", role: models.RoleService, id: "5", err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.role == models.RoleService && tt.id == "5" {
				suite.uc.On("Start", mock.Anything, models.Actor{UserID: 1, Role: models.RoleService}, 5).Once().
					Return(models.Mark{ID: 5, MarkStatusID: models.InProgressStatus}, tt.err)
			}

			w := suite.do("POST", "/marks/"+tt.id+"/start", nil, "", tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *OrganizationsSuite) TestResolve() {
	tests := []struct {
		name       string
		role       models.Role
		withPhoto  bool
		err        error
		statusCode int
	}{
		{name: "Ok200", role: models.RoleService, withPhoto: true, statusCode: http.StatusOK},
		{name: "Err400NoPhotos", role: models.RoleService, statusCode: http.StatusBadRequest},
		{name: "Err403User", role: models.RoleUser, withPhoto: true, statusCode: http.StatusForbidden},
		{name: "Err409", role: models.RoleService, withPhoto: true, err: usecase.ErrConflict, statusCode: http.StatusConflict},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.role == models.RoleService && tt.withPhoto {
				suite.uc.On("Resolve", mock.Anything, models.Actor{UserID: 1, Role: models.RoleService}, 5, "готово", mock.Anything).Once().
					Return(models.Mark{ID: 5, MarkStatusID: models.UnderReviewStatus}, tt.err)
			}

			var b bytes.Buffer
			mpw := multipart.NewWriter(&b)
			suite.Require().NoError(mpw.WriteField("comment", "готово"))
			if tt.withPhoto {
				fw, err := mpw.CreateFormFile("photos", "after.jpg")
				suite.Require().NoError(err)
				_, err = fw.Write(gofakeit.ImageJpeg(10, 10))
				suite.Require().NoError(err)
			}
			suite.Require().NoError(mpw.Close())

			w := suite.do("POST", "/marks/5/resolve", b.Bytes(), mpw.FormDataContentType(), tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *OrganizationsSuite) TestAssign() {
	tests := []struct {
		name       string
		role       models.Role
		body       string
		err        error
		statusCode int
	}{
		{name: "Ok200Moderator", role: models.RoleModerator, body: `{"organization_id":3}`, statusCode: http.StatusOK},
		{name: "Ok200Admin", role: models.RoleAdmin, body: `{"organization_id":3}`, statusCode: http.StatusOK},
		{name: "Err400", role: models.RoleAdmin, body: `{}`, statusCode: http.StatusBadRequest},
		{name: "Err403Service", role: models.RoleService, body: `{"organization_id":3}`, statusCode: http.StatusForbidden},
		{name: "Err404", role: models.RoleAdmin, body: `{"organization_id":3}`, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409", role: models.RoleAdmin, body: `{"organization_id":3}`, err: usecase.ErrConflict, statusCode: http.StatusConflict},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.role != models.RoleService && tt.body != `{}` {
				suite.uc.On("Assign", mock.Anything, 5, 3).Once().Return(models.Mark{ID: 5}, tt.err)
			}

			w := suite.do("PATCH", "/marks/5/assign", []byte(tt.body), "application/json", tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

// withoutID is the responsibility as the handler builds it from the request.
func withoutID(r models.OrganizationResponsibility) models.OrganizationResponsibility {
	r.ID = 0
	return r
}
