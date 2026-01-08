package usersrest_test

import (
	"errors"
	"net/http/httptest"
	"testing"

	usersrest "github.com/PritOriginal/problem-map-server/internal/handler/users"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/pkg/handlers"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	mock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type UsersSuite struct {
	suite.Suite
	r  *chi.Mux
	uc *usersrest.MockUsers
}

func (suite *UsersSuite) SetupSuite() {
	suite.uc = usersrest.NewMockUsers(suite.T())

	log := slogdiscard.NewDiscardLogger()
	validate := validator.New()
	baseHandler := &handlers.BaseHandler{Log: log, Validate: validate}

	suite.r = chi.NewRouter()

	usersrest.Register(suite.r, suite.uc, baseHandler)
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
			errGetUserById: storage.ErrNotFound,
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
					Return(models.User{}, tt.errGetUserById)
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users/"+tt.id, nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
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
			suite.uc.On("GetUsers", mock.Anything).Once().
				Return([]models.User{}, tt.errGetUsers)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/users", nil)

			suite.r.ServeHTTP(w, req)

			suite.Equal(tt.statusCode, w.Code)
		})
	}
}
