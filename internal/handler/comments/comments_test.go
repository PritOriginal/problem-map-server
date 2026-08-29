package commentsrest_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	commentsrest "github.com/PritOriginal/problem-map-server/internal/handler/comments"
	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	jwt "github.com/appleboy/gin-jwt/v3"
	"github.com/gin-gonic/gin"
	"github.com/guregu/null/v6"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const jwtKey = "1234"

type CommentsSuite struct {
	suite.Suite
	r  *gin.Engine
	uc *commentsrest.MockComments
}

func (suite *CommentsSuite) SetupTest() {
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{Key: []byte(jwtKey)})
	suite.Require().NoError(err)
	suite.Require().NoError(authMiddleware.MiddlewareInit())

	suite.uc = commentsrest.NewMockComments(suite.T())

	gin.SetMode(gin.TestMode)
	suite.r = gin.New()
	commentsrest.Register(suite.r, slogdiscard.NewDiscardLogger(), authMiddleware, suite.uc)
}

func TestComments(t *testing.T) {
	suite.Run(t, new(CommentsSuite))
}

// bearer returns an Authorization header value for user id 1 with the role.
func (suite *CommentsSuite) bearer(role models.Role) string {
	accessToken, err := token.CreateToken(1*time.Minute, 1, string(role), jwtKey)
	suite.Require().NoError(err)
	return "Bearer " + accessToken
}

// viewerIs matches a context whose viewer (models.ViewerFromContext) is id.
func viewerIs(id int) any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		return models.ViewerFromContext(ctx) == id
	})
}

func TestAddCommentRequest_MaxLenMatchesModel(t *testing.T) {
	// The binding tag cannot reference models.MaxCommentBodyLen; keep them in sync.
	for _, typ := range []reflect.Type{
		reflect.TypeFor[commentsrest.AddCommentRequest](),
		reflect.TypeFor[commentsrest.UpdateCommentRequest](),
	} {
		f, ok := typ.FieldByName("Body")
		if !ok {
			t.Fatalf("%s has no Body field", typ)
		}
		if !strings.Contains(f.Tag.Get("binding"), "max=2000") || models.MaxCommentBodyLen != 2000 {
			t.Fatalf("%s: binding tag %q does not match models.MaxCommentBodyLen=%d", typ, f.Tag.Get("binding"), models.MaxCommentBodyLen)
		}
	}
}

func (suite *CommentsSuite) TestGetComments() {
	tests := []struct {
		name       string
		id         string
		query      string
		withToken  bool
		wantViewer int
		wantPage   *models.Pagination
		errList    error
		statusCode int
	}{
		{name: "Ok200Anonymous", id: "1", wantPage: &models.Pagination{Limit: 100}, statusCode: http.StatusOK},
		{name: "Ok200ViewerFromToken", id: "1", withToken: true, wantViewer: 1, wantPage: &models.Pagination{Limit: 100}, statusCode: http.StatusOK},
		{name: "Ok200Paginated", id: "1", query: "?limit=5&offset=10", wantPage: &models.Pagination{Limit: 5, Offset: 10}, statusCode: http.StatusOK},
		{name: "Err400BadId", id: "a", statusCode: http.StatusBadRequest},
		{name: "Err400BadLimit", id: "1", query: "?limit=0", statusCode: http.StatusBadRequest},
		{name: "Err404", id: "1", wantPage: &models.Pagination{Limit: 100}, errList: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", id: "1", wantPage: &models.Pagination{Limit: 100}, errList: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantPage != nil {
				suite.uc.On("ListComments", viewerIs(tt.wantViewer), 1, *tt.wantPage).Once().
					Return(models.Page[models.Comment]{Items: []models.Comment{{ID: 1, IsMine: tt.withToken}}, Total: 1}, tt.errList)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/marks/"+tt.id+"/comments"+tt.query, nil)
			if tt.withToken {
				req.Header.Set("Authorization", suite.bearer(models.RoleUser))
			}
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				suite.Contains(w.Body.String(), `"meta":{"limit":`)
			}
		})
	}
}

func (suite *CommentsSuite) TestAddComment() {
	tests := []struct {
		name       string
		id         string
		body       string
		noToken    bool
		want       *models.Comment
		errAdd     error
		statusCode int
	}{
		{name: "Ok201", id: "1", body: `{"body":"hello"}`, want: &models.Comment{MarkID: 1, UserID: 1, Body: "hello"}, statusCode: http.StatusCreated},
		{name: "Ok201Reply", id: "1", body: `{"body":"hello","parent_id":7}`, want: &models.Comment{MarkID: 1, UserID: 1, Body: "hello", ParentID: null.IntFrom(7)}, statusCode: http.StatusCreated},
		{name: "Err400BadId", id: "a", body: `{"body":"hello"}`, statusCode: http.StatusBadRequest},
		{name: "Err400BadJSON", id: "1", body: `{`, statusCode: http.StatusBadRequest},
		{name: "Err400NoBody", id: "1", body: `{}`, statusCode: http.StatusBadRequest},
		{name: "Err400TooLong", id: "1", body: `{"body":"` + strings.Repeat("A", models.MaxCommentBodyLen+1) + `"}`, statusCode: http.StatusBadRequest},
		{name: "Err400BadParent", id: "1", body: `{"body":"hello","parent_id":0}`, statusCode: http.StatusBadRequest},
		{name: "Err400Blank", id: "1", body: `{"body":"   "}`, want: &models.Comment{MarkID: 1, UserID: 1, Body: "   "}, errAdd: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err401NoToken", id: "1", body: `{"body":"hello"}`, noToken: true, statusCode: http.StatusUnauthorized},
		{name: "Err404", id: "1", body: `{"body":"hello"}`, want: &models.Comment{MarkID: 1, UserID: 1, Body: "hello"}, errAdd: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409Duplicate", id: "1", body: `{"body":"hello"}`, want: &models.Comment{MarkID: 1, UserID: 1, Body: "hello"}, errAdd: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err429Limit", id: "1", body: `{"body":"hello"}`, want: &models.Comment{MarkID: 1, UserID: 1, Body: "hello"}, errAdd: usecase.ErrTooManyRequests, statusCode: http.StatusTooManyRequests},
		{name: "Err500", id: "1", body: `{"body":"hello"}`, want: &models.Comment{MarkID: 1, UserID: 1, Body: "hello"}, errAdd: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.want != nil {
				created := *tt.want
				created.ID = 10
				created.IsMine = true
				suite.uc.On("AddComment", viewerIs(1), *tt.want).Once().Return(created, tt.errAdd)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/marks/"+tt.id+"/comments", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(models.RoleUser))
			}
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusCreated {
				suite.Contains(w.Body.String(), `"comment_id":10`)
				suite.Contains(w.Body.String(), `"is_mine":true`)
			}
		})
	}
}

func (suite *CommentsSuite) TestUpdateComment() {
	tests := []struct {
		name       string
		id         string
		body       string
		role       models.Role
		noToken    bool
		callUC     bool
		errUpdate  error
		statusCode int
	}{
		{name: "Ok200", id: "1", body: `{"body":"fixed"}`, role: models.RoleUser, callUC: true, statusCode: http.StatusOK},
		{name: "Err400BadId", id: "a", body: `{"body":"fixed"}`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400BadJSON", id: "1", body: `{`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400NoBody", id: "1", body: `{}`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err400TooLong", id: "1", body: `{"body":"` + strings.Repeat("A", models.MaxCommentBodyLen+1) + `"}`, role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err401NoToken", id: "1", body: `{"body":"fixed"}`, noToken: true, statusCode: http.StatusUnauthorized},
		{name: "Err403Stranger", id: "1", body: `{"body":"fixed"}`, role: models.RoleModerator, callUC: true, errUpdate: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err404", id: "1", body: `{"body":"fixed"}`, role: models.RoleUser, callUC: true, errUpdate: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409WindowExpired", id: "1", body: `{"body":"fixed"}`, role: models.RoleUser, callUC: true, errUpdate: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err500", id: "1", body: `{"body":"fixed"}`, role: models.RoleUser, callUC: true, errUpdate: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.callUC {
				suite.uc.On("UpdateComment", viewerIs(1), models.Actor{UserID: 1, Role: tt.role}, 1, "fixed").Once().
					Return(models.Comment{ID: 1, Body: "fixed", IsMine: true}, tt.errUpdate)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("PATCH", "/comments/"+tt.id, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(tt.role))
			}
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
		})
	}
}

func (suite *CommentsSuite) TestDeleteComment() {
	tests := []struct {
		name       string
		id         string
		role       models.Role
		noToken    bool
		callUC     bool
		errDelete  error
		statusCode int
	}{
		{name: "Ok200Owner", id: "1", role: models.RoleUser, callUC: true, statusCode: http.StatusOK},
		{name: "Ok200Moderator", id: "1", role: models.RoleModerator, callUC: true, statusCode: http.StatusOK},
		{name: "Err400BadId", id: "a", role: models.RoleUser, statusCode: http.StatusBadRequest},
		{name: "Err401NoToken", id: "1", noToken: true, statusCode: http.StatusUnauthorized},
		{name: "Err403", id: "1", role: models.RoleUser, callUC: true, errDelete: usecase.ErrForbidden, statusCode: http.StatusForbidden},
		{name: "Err404", id: "1", role: models.RoleUser, callUC: true, errDelete: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409AlreadyDeleted", id: "1", role: models.RoleUser, callUC: true, errDelete: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err500", id: "1", role: models.RoleUser, callUC: true, errDelete: errors.New(""), statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.callUC {
				suite.uc.On("DeleteComment", mock.Anything, models.Actor{UserID: 1, Role: tt.role}, 1).Once().Return(tt.errDelete)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("DELETE", "/comments/"+tt.id, nil)
			if !tt.noToken {
				req.Header.Set("Authorization", suite.bearer(tt.role))
			}
			suite.r.ServeHTTP(w, req)

			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode == http.StatusOK {
				suite.Contains(w.Body.String(), `"comment_id":1`)
			}
		})
	}
}
