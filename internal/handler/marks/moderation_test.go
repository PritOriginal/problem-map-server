package marksrest_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	"github.com/PritOriginal/problem-map-server/internal/handler/handlertest"
	marksrest "github.com/PritOriginal/problem-map-server/internal/handler/marks"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/responses"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
)

var errInternal = errors.New("boom")

// doAs performs a JSON request with the bearer of role (empty role means
// anonymous).
func (suite *MarksSuite) doAs(method, path, body string, role models.Role) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		req.Header.Set("Authorization", suite.bearer(role))
	}
	w := httptest.NewRecorder()
	suite.r.ServeHTTP(w, req)
	return w
}

func (suite *MarksSuite) TestSetHidden() {
	hidden := models.Mark{ID: 5, UserID: 3, Hidden: true}

	tests := []struct {
		name       string
		id         string
		body       string
		role       models.Role
		wantHidden *bool
		err        error
		statusCode int
	}{
		{name: "Anonymous401", id: "5", body: `{"hidden":true}`, statusCode: http.StatusUnauthorized},
		{name: "User403", id: "5", body: `{"hidden":true}`, role: models.RoleUser, statusCode: http.StatusForbidden},
		{name: "Service403", id: "5", body: `{"hidden":true}`, role: models.RoleService, statusCode: http.StatusForbidden},
		{name: "Moderator200Hide", id: "5", body: `{"hidden":true}`, role: models.RoleModerator, wantHidden: ptr(true), statusCode: http.StatusOK},
		{name: "Admin200Show", id: "5", body: `{"hidden":false}`, role: models.RoleAdmin, wantHidden: ptr(false), statusCode: http.StatusOK},
		{name: "Err400MissingHidden", id: "5", body: `{}`, role: models.RoleModerator, statusCode: http.StatusBadRequest},
		{name: "Err400NotJSON", id: "5", body: `{`, role: models.RoleModerator, statusCode: http.StatusBadRequest},
		{name: "Err400Id", id: "abc", body: `{"hidden":true}`, role: models.RoleModerator, statusCode: http.StatusBadRequest},
		{name: "Err404", id: "5", body: `{"hidden":true}`, role: models.RoleModerator, wantHidden: ptr(true), err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err500", id: "5", body: `{"hidden":true}`, role: models.RoleModerator, wantHidden: ptr(true), err: errInternal, statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantHidden != nil {
				suite.uc.On("SetHidden", mock.Anything, models.Actor{UserID: 1, Role: tt.role}, 5, *tt.wantHidden).Once().Return(hidden, tt.err)
			}

			w := suite.doAs(http.MethodPatch, "/marks/"+tt.id+"/hidden", tt.body, tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusOK {
				return
			}

			var resp responses.Response[marksrest.UpdateMarkResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Equal(5, resp.Payload.Mark.ID)
			suite.True(resp.Payload.Mark.Hidden)
		})
	}
}

func (suite *MarksSuite) TestMergeInto() {
	merged := models.Mark{ID: 5, UserID: 3, MarkStatusID: models.DuplicateStatus, MergedIntoID: null.IntFrom(2)}

	tests := []struct {
		name       string
		path       string
		role       models.Role
		callUC     bool
		err        error
		statusCode int
	}{
		{name: "Anonymous401", path: "/marks/5/merge-into/2", statusCode: http.StatusUnauthorized},
		{name: "User403", path: "/marks/5/merge-into/2", role: models.RoleUser, statusCode: http.StatusForbidden},
		{name: "Moderator200", path: "/marks/5/merge-into/2", role: models.RoleModerator, callUC: true, statusCode: http.StatusOK},
		{name: "Admin200", path: "/marks/5/merge-into/2", role: models.RoleAdmin, callUC: true, statusCode: http.StatusOK},
		{name: "Err400Id", path: "/marks/abc/merge-into/2", role: models.RoleModerator, statusCode: http.StatusBadRequest},
		{name: "Err400TargetId", path: "/marks/5/merge-into/abc", role: models.RoleModerator, statusCode: http.StatusBadRequest},
		{name: "Err400SameMark", path: "/marks/5/merge-into/2", role: models.RoleModerator, callUC: true, err: usecase.ErrInvalidArgument, statusCode: http.StatusBadRequest},
		{name: "Err404", path: "/marks/5/merge-into/2", role: models.RoleModerator, callUC: true, err: usecase.ErrNotFound, statusCode: http.StatusNotFound},
		{name: "Err409NotActive", path: "/marks/5/merge-into/2", role: models.RoleModerator, callUC: true, err: usecase.ErrConflict, statusCode: http.StatusConflict},
		{name: "Err500", path: "/marks/5/merge-into/2", role: models.RoleModerator, callUC: true, err: errInternal, statusCode: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.callUC {
				suite.uc.On("MergeInto", mock.Anything, models.Actor{UserID: 1, Role: tt.role}, 5, 2).Once().Return(merged, tt.err)
			}

			w := suite.doAs(http.MethodPost, tt.path, "", tt.role)
			handlertest.AssertResponse(suite.T(), w, tt.statusCode)
			if tt.statusCode != http.StatusOK {
				return
			}

			var resp responses.Response[marksrest.MergeIntoResponse]
			suite.Require().NoError(json.Unmarshal(w.Body.Bytes(), &resp))
			suite.Equal(2, resp.Payload.MergedIntoId)
			suite.Equal(models.DuplicateStatus, resp.Payload.Mark.MarkStatusID)
			suite.Equal(int64(2), resp.Payload.Mark.MergedIntoID.ValueOrZero())
		})
	}
}

// TestGetMarkById_HiddenIs404: the handler passes the hidden-mark
// ErrNotFound of the usecase through as a plain 404.
func (suite *MarksSuite) TestGetMarkById_HiddenIs404() {
	suite.uc.On("GetMarkById", mock.Anything, 5).Once().Return(models.Mark{}, usecase.ErrNotFound)

	w := suite.doAs(http.MethodGet, "/marks/5", "", "")
	handlertest.AssertResponse(suite.T(), w, http.StatusNotFound)
}
