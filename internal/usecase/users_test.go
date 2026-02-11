package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type UsersSuite struct {
	suite.Suite
	uc        *usecase.Users
	log       *slog.Logger
	usersRepo *usecase.MockUsersRepository
}

func (suite *UsersSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.usersRepo = usecase.NewMockUsersRepository(suite.T())
	suite.uc = usecase.NewUsers(suite.log, usecase.UsersRepositories{
		Users: suite.usersRepo,
	})
}

func TestUsers(t *testing.T) {
	suite.Run(t, new(UsersSuite))
}

func (suite *UsersSuite) TestGetUserById() {
	tests := []struct {
		name        string
		getUserById method[models.User]
	}{
		{
			name: "Ok",
			getUserById: method[models.User]{
				data: models.User{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getUserById: method[models.User]{
				data: models.User{},
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.usersRepo.On("GetUserById", mock.Anything, mock.AnythingOfType("int")).Once().
					Return(tt.getUserById.data, tt.getUserById.err)
				if tt.getUserById.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetUserById(context.Background(), 1)

			if tt.getUserById.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *UsersSuite) TestGetUsers() {
	tests := []struct {
		name     string
		getUsers method[[]models.User]
	}{
		{
			name: "Ok",
			getUsers: method[[]models.User]{
				data: []models.User{},
				err:  nil,
			},
		},
		{
			name: "Err",
			getUsers: method[[]models.User]{
				data: nil,
				err:  errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.usersRepo.On("GetUsers", mock.Anything).Once().
					Return(tt.getUsers.data, tt.getUsers.err)
				if tt.getUsers.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetUsers(context.Background())

			if tt.getUsers.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}
