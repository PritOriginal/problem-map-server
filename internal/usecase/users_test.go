package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
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

func (suite *UsersSuite) SetupTest() {
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
			name: "ErrRepo",
			getUserById: method[models.User]{
				data: models.User{},
				err:  errRepo,
			},
		},
		{
			name: "ErrNotFound",
			getUserById: method[models.User]{
				data: models.User{},
				err:  repository.ErrNotFound,
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
				assertRepoErr(&suite.Suite, gotErr, tt.getUserById.err)
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
				err:  errRepo,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			func() {
				suite.usersRepo.On("GetUsers", mock.Anything, models.Pagination{}).Once().
					Return(models.Page[models.User]{Items: tt.getUsers.data}, tt.getUsers.err)
				if tt.getUsers.err != nil {
					return
				}
			}()

			_, gotErr := suite.uc.GetUsers(context.Background())

			if tt.getUsers.err == nil {
				suite.NoError(gotErr)
			} else {
				assertRepoErr(&suite.Suite, gotErr, tt.getUsers.err)
			}
			suite.usersRepo.AssertExpectations(suite.T())
		})
	}
}

func (suite *UsersSuite) TestGetUserStats() {
	tests := []struct {
		name  string
		stats method[models.UserStats]
	}{
		{name: "Ok", stats: method[models.UserStats]{data: models.UserStats{Rating: 5, MarksTotal: 2, ChecksCorrect: 1}}},
		{name: "ErrNotFound", stats: method[models.UserStats]{err: repository.ErrNotFound}},
		{name: "ErrRepo", stats: method[models.UserStats]{err: errRepo}},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.usersRepo.On("GetUserStats", mock.Anything, 1).Once().Return(tt.stats.data, tt.stats.err)

			got, err := suite.uc.GetUserStats(context.Background(), 1)

			if tt.stats.err == nil {
				suite.NoError(err)
				suite.Equal(tt.stats.data, got)
			} else {
				assertRepoErr(&suite.Suite, err, tt.stats.err)
			}
		})
	}
}

func (suite *UsersSuite) TestListLeaderboard() {
	page := models.Page[models.User]{Items: []models.User{{Id: 2, Rating: 10}, {Id: 1, Rating: 3}}, Total: 2}

	suite.usersRepo.On("GetLeaderboard", mock.Anything, models.Pagination{Limit: 10}).Once().Return(page, nil)
	got, err := suite.uc.ListLeaderboard(context.Background(), models.Pagination{Limit: 10})
	suite.NoError(err)
	suite.Equal(page, got)

	suite.usersRepo.On("GetLeaderboard", mock.Anything, models.Pagination{Limit: 10}).Once().Return(models.Page[models.User]{}, errRepo)
	_, err = suite.uc.ListLeaderboard(context.Background(), models.Pagination{Limit: 10})
	suite.ErrorIs(err, errRepo)

	_, err = suite.uc.ListLeaderboard(context.Background(), models.Pagination{Limit: models.MaxLimit + 1})
	suite.ErrorIs(err, usecase.ErrInvalidArgument)
}

func (suite *UsersSuite) TestListRatingEvents() {
	page := models.Page[models.RatingEvent]{Items: []models.RatingEvent{{ID: 1, UserID: 7, Delta: 2}}, Total: 1}

	tests := []struct {
		name      string
		requester usecase.Requester
		userId    int
		repoErr   error
		wantErr   error
	}{
		{name: "Owner", requester: usecase.Requester{ID: 7, Role: models.RoleUser}, userId: 7},
		{name: "Moderator", requester: usecase.Requester{ID: 1, Role: models.RoleModerator}, userId: 7},
		{name: "Admin", requester: usecase.Requester{ID: 1, Role: models.RoleAdmin}, userId: 7},
		{name: "OtherUserForbidden", requester: usecase.Requester{ID: 1, Role: models.RoleUser}, userId: 7, wantErr: usecase.ErrForbidden},
		{name: "ErrRepo", requester: usecase.Requester{ID: 7, Role: models.RoleUser}, userId: 7, repoErr: errRepo, wantErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !errors.Is(tt.wantErr, usecase.ErrForbidden) {
				suite.usersRepo.On("GetRatingEvents", mock.Anything, tt.userId, models.Pagination{Limit: 10}).Once().
					Return(page, tt.repoErr)
			}

			got, err := suite.uc.ListRatingEvents(context.Background(), tt.requester, tt.userId, models.Pagination{Limit: 10})

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
			suite.Equal(page, got)
		})
	}

	_, err := suite.uc.ListRatingEvents(context.Background(), usecase.Requester{ID: 7}, 7, models.Pagination{Offset: -1})
	suite.ErrorIs(err, usecase.ErrInvalidArgument)
}
