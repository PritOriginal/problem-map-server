package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/storage"
	"github.com/PritOriginal/problem-map-server/internal/usecase"

	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	passwordUtils "github.com/PritOriginal/problem-map-server/pkg/password"
	"github.com/PritOriginal/problem-map-server/pkg/token"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type AuthSuite struct {
	suite.Suite
	uc        *usecase.Auth
	log       *slog.Logger
	usersRepo *usecase.MockUsersRepository
	authCfg   config.AuthConfing
}

func (suite *AuthSuite) SetupSuite() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.usersRepo = usecase.NewMockUsersRepository(suite.T())
	cfg := config.MustLoadPath("../../configs/config-tests.yaml")
	suite.authCfg = cfg.Auth
	suite.uc = usecase.NewAuth(suite.log, suite.usersRepo, cfg.Auth)
}

func TestAuth(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

func (suite *AuthSuite) TestSignUp() {
	tests := []struct {
		name                     string
		GetUserByUsernameWantErr bool
		AddUserWantErr           bool
	}{
		{
			name:                     "Ok",
			GetUserByUsernameWantErr: false,
			AddUserWantErr:           false,
		},
		{
			name:                     "ErrGetUserByUsername",
			GetUserByUsernameWantErr: true,
			AddUserWantErr:           false,
		},
		{
			name:                     "ErrAddUser",
			GetUserByUsernameWantErr: false,
			AddUserWantErr:           true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			usersRepoCall := suite.usersRepo.On("GetUserByUsername", mock.Anything, mock.AnythingOfType("string")).Once()
			if !tt.GetUserByUsernameWantErr {
				usersRepoCall.Return(models.User{}, storage.ErrNotFound)

				usersRepoCall2 := suite.usersRepo.On("AddUser", mock.Anything, mock.Anything).Once()
				if !tt.AddUserWantErr {
					usersRepoCall2.Return(int64(1), nil)
				} else {
					usersRepoCall2.Return(int64(0), errors.New(""))
				}
			} else {
				usersRepoCall.Return(models.User{}, errors.New(""))
			}

			_, gotErr := suite.uc.SignUp(context.Background(), "name", "username", "password")

			if !tt.GetUserByUsernameWantErr && !tt.AddUserWantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *AuthSuite) TestSignIn() {
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
	password := "password"
	passwordHash, err := passwordUtils.HashPassword(password)
	suite.NoError(err)

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			usersRepoCall := suite.usersRepo.On("GetUserByUsername", mock.Anything, mock.AnythingOfType("string")).Once()
			if !tt.wantErr {
				usersRepoCall.Return(models.User{
					PasswordHash: passwordHash,
				}, nil)
			} else {
				usersRepoCall.Return(models.User{}, errors.New(""))
			}

			_, _, gotErr := suite.uc.SignIn(context.Background(), "username", "password")

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}

func (suite *AuthSuite) TestRefreshTokens() {
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

	userId := 1
	refreshToken, err := token.CreateToken(suite.authCfg.JWT.Refresh.ExpiredIn, userId, suite.authCfg.JWT.Refresh.Key)
	suite.NoError(err)

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			usersRepoCall := suite.usersRepo.On("GetUserById", mock.Anything, mock.AnythingOfType("int")).Once()
			if !tt.wantErr {
				usersRepoCall.Return(models.User{Id: userId}, nil)
			} else {
				usersRepoCall.Return(models.User{}, errors.New(""))
			}

			_, _, gotErr := suite.uc.RefreshTokens(context.Background(), refreshToken)

			if !tt.wantErr {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
		})
	}
}
