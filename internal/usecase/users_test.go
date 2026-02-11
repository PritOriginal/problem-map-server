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
		name    string
		wantErr bool
	}{
		{
			name:    "Ok",
			wantErr: false,
		},
		{
			name:    "Err",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			usersRepoCall := suite.usersRepo.On("GetUserById", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				usersRepoCall.Return(models.User{}, nil)
			} else {
				usersRepoCall.Return(models.User{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetUserById(context.Background(), 1)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *UsersSuite) TestGetUsers() {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "Ok",
			wantErr: false,
		},
		{
			name:    "Err",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			usersRepoCall := suite.usersRepo.On("GetUsers", mock.Anything).Once()
			if !tt.wantErr {
				usersRepoCall.Return([]models.User{}, nil)
			} else {
				usersRepoCall.Return([]models.User{}, errors.New(""))
			}

			_, gotErr := suite.uc.GetUsers(context.Background())

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}
