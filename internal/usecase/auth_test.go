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

type method[T any] struct {
	data T
	err  error
}

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
	suite.uc = usecase.NewAuth(suite.log, cfg.Auth, usecase.AuthRepositories{
		Users: suite.usersRepo,
	})
}

func TestAuth(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

func (suite *AuthSuite) TestSignUp() {
	tests := []struct {
		name           string
		getUserByLogin method[models.User]
		addUser        method[int64]
	}{
		{
			name: "Ok",
			getUserByLogin: method[models.User]{
				err: storage.ErrNotFound,
			},
			addUser: method[int64]{
				err: nil,
			},
		},
		{
			name: "ErrGetUserByLogin",
			getUserByLogin: method[models.User]{
				err: errors.New(""),
			},
			addUser: method[int64]{
				err: nil,
			},
		},
		{
			name: "ErrAddUser",
			getUserByLogin: method[models.User]{
				err: storage.ErrNotFound,
			},
			addUser: method[int64]{
				err: errors.New(""),
			},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.usersRepo.On("GetUserByLogin", mock.Anything, mock.AnythingOfType("string")).Once().
					Return(tt.getUserByLogin.data, tt.getUserByLogin.err)
				if tt.getUserByLogin.err != storage.ErrNotFound {
					return
				}

				suite.usersRepo.On("AddUser", mock.Anything, mock.Anything).Once().
					Return(tt.addUser.data, tt.addUser.err)
				if tt.addUser.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.SignUp(context.Background(), "username", "login", "password")

			if tt.getUserByLogin.err == storage.ErrNotFound && tt.addUser.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}

			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *AuthSuite) TestSignIn() {
	password := "password"
	passwordHash, err := passwordUtils.HashPassword(password)
	suite.NoError(err)

	tests := []struct {
		name           string
		getUserByLogin method[models.User]
	}{
		{
			name: "Ok",
			getUserByLogin: method[models.User]{
				data: models.User{
					PasswordHash: passwordHash,
				},
				err: nil,
			},
		},
		{
			name: "Err",
			getUserByLogin: method[models.User]{
				err: errors.New(""),
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.usersRepo.On("GetUserByLogin", mock.Anything, mock.AnythingOfType("string")).Once().
					Return(tt.getUserByLogin.data, tt.getUserByLogin.err)
				if tt.getUserByLogin.err != nil {
					return
				}
			}()

			_, _, gotErr := suite.uc.SignIn(context.Background(), "login", "password")

			if tt.getUserByLogin.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *AuthSuite) TestRefreshTokens() {
	userId := 1
	refreshToken, err := token.CreateToken(suite.authCfg.JWT.Refresh.ExpiredIn, userId, suite.authCfg.JWT.Refresh.Key)
	suite.NoError(err)

	tests := []struct {
		name        string
		getUserById method[models.User]
	}{
		{
			name: "Ok",
			getUserById: method[models.User]{
				data: models.User{
					Id: userId,
				},
				err: nil,
			},
		},
		{
			name: "Err",
			getUserById: method[models.User]{
				err: errors.New(""),
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

			_, _, gotErr := suite.uc.RefreshTokens(context.Background(), refreshToken)

			if tt.getUserById.err == nil {
				suite.NoError(gotErr)
			} else {
				suite.NotNil(gotErr)
			}
			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}
