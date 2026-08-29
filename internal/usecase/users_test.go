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
	passwordUtils "github.com/PritOriginal/problem-map-server/pkg/password"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type UsersSuite struct {
	suite.Suite
	uc        *usecase.Users
	log       *slog.Logger
	usersRepo *usecase.MockUsersRepository
	refresh   *usecase.MockRefreshStore
	versions  *usecase.MockAuthVersionStore
}

func (suite *UsersSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.usersRepo = usecase.NewMockUsersRepository(suite.T())
	suite.refresh = usecase.NewMockRefreshStore(suite.T())
	suite.versions = usecase.NewMockAuthVersionStore(suite.T())
	suite.uc = usecase.NewUsers(suite.log, usecase.UsersRepositories{
		Users:         suite.usersRepo,
		RefreshTokens: suite.refresh,
		AuthVersions:  suite.versions,
	})
}

// expectRevokeAll registers the session revocation that follows a password
// or role change.
func (suite *UsersSuite) expectRevokeAll(userID int, deleteAllErr, incrErr error) {
	suite.refresh.On("DeleteAllRefresh", mock.Anything, userID).Once().Return(deleteAllErr)
	suite.versions.On("IncrAuthVersion", mock.Anything, userID).Once().Return(int64(1), incrErr)
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

func (suite *UsersSuite) TestChangePassword() {
	const userID = 1
	oldHash, err := passwordUtils.HashPassword("old-password")
	suite.Require().NoError(err)

	tests := []struct {
		name           string
		oldPassword    string
		newPassword    string
		getUserById    *method[models.User]
		updatePassword *method[struct{}]
		revokeErr      error
		wantErr        error
	}{
		{
			name: "Ok", oldPassword: "old-password", newPassword: "new-password",
			getUserById:    &method[models.User]{data: models.User{Id: userID, PasswordHash: oldHash}},
			updatePassword: &method[struct{}]{},
		},
		{
			name: "OkStoresUnavailable", oldPassword: "old-password", newPassword: "new-password",
			getUserById:    &method[models.User]{data: models.User{Id: userID, PasswordHash: oldHash}},
			updatePassword: &method[struct{}]{},
			revokeErr:      errStore,
		},
		{
			name: "ErrInvalidArgumentShort", oldPassword: "old-password", newPassword: "short",
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name: "ErrForbiddenWrongOld", oldPassword: "wrong-password", newPassword: "new-password",
			getUserById: &method[models.User]{data: models.User{Id: userID, PasswordHash: oldHash}},
			wantErr:     usecase.ErrForbidden,
		},
		{
			name: "ErrNotFound", oldPassword: "old-password", newPassword: "new-password",
			getUserById: &method[models.User]{err: repository.ErrNotFound},
			wantErr:     usecase.ErrNotFound,
		},
		{
			name: "ErrRepoGet", oldPassword: "old-password", newPassword: "new-password",
			getUserById: &method[models.User]{err: errRepo},
			wantErr:     errRepo,
		},
		{
			name: "ErrRepoUpdate", oldPassword: "old-password", newPassword: "new-password",
			getUserById:    &method[models.User]{data: models.User{Id: userID, PasswordHash: oldHash}},
			updatePassword: &method[struct{}]{err: errRepo},
			wantErr:        errRepo,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.getUserById != nil {
				suite.usersRepo.On("GetUserById", mock.Anything, userID).Once().Return(tt.getUserById.data, tt.getUserById.err)
			}
			if tt.updatePassword != nil {
				suite.usersRepo.On("UpdatePassword", mock.Anything, userID, mock.MatchedBy(func(hash string) bool {
					return passwordUtils.CheckPasswordHash(tt.newPassword, hash)
				})).Once().Return(tt.updatePassword.err)
			}
			if tt.wantErr == nil {
				suite.expectRevokeAll(userID, tt.revokeErr, tt.revokeErr)
			}

			err := suite.uc.ChangePassword(context.Background(), userID, tt.oldPassword, tt.newPassword)

			if tt.wantErr == nil {
				suite.NoError(err)
			} else {
				suite.ErrorIs(err, tt.wantErr)
			}
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

func (suite *UsersSuite) TestSetRole() {
	const userID = 1
	const adminID = 9

	tests := []struct {
		name       string
		actorID    int
		role       models.Role
		admins     *method[int64]
		updateRole *method[struct{}]
		revokeErr  error
		wantErr    error
	}{
		{name: "OkModerator", actorID: adminID, role: models.RoleModerator, updateRole: &method[struct{}]{}},
		{name: "OkUser", actorID: adminID, role: models.RoleUser, updateRole: &method[struct{}]{}},
		{name: "OkAdminStoresUnavailable", actorID: adminID, role: models.RoleAdmin, updateRole: &method[struct{}]{}, revokeErr: errStore},
		{name: "OkSelfKeepsAdmin", actorID: userID, role: models.RoleAdmin, updateRole: &method[struct{}]{}},
		{name: "OkSelfDemoteWithOtherAdmins", actorID: userID, role: models.RoleUser, admins: &method[int64]{data: 2}, updateRole: &method[struct{}]{}},
		{name: "ErrForbiddenLastAdminSelfDemote", actorID: userID, role: models.RoleModerator, admins: &method[int64]{data: 1}, wantErr: usecase.ErrForbidden},
		{name: "ErrRepoCountAdmins", actorID: userID, role: models.RoleUser, admins: &method[int64]{err: errRepo}, wantErr: errRepo},
		{name: "ErrInvalidArgumentUnknownRole", actorID: adminID, role: "root", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrInvalidArgumentEmptyRole", actorID: adminID, role: "", wantErr: usecase.ErrInvalidArgument},
		{name: "ErrNotFound", actorID: adminID, role: models.RoleModerator, updateRole: &method[struct{}]{err: repository.ErrNotFound}, wantErr: usecase.ErrNotFound},
		{name: "ErrRepo", actorID: adminID, role: models.RoleModerator, updateRole: &method[struct{}]{err: errRepo}, wantErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.admins != nil {
				suite.usersRepo.On("CountByRole", mock.Anything, models.RoleAdmin).Once().Return(tt.admins.data, tt.admins.err)
			}
			if tt.updateRole != nil {
				suite.usersRepo.On("UpdateRole", mock.Anything, userID, tt.role).Once().Return(tt.updateRole.err)
			}
			if tt.wantErr == nil {
				suite.expectRevokeAll(userID, tt.revokeErr, tt.revokeErr)
			}

			err := suite.uc.SetRole(context.Background(), tt.actorID, userID, tt.role)

			if tt.wantErr == nil {
				suite.NoError(err)
			} else {
				suite.ErrorIs(err, tt.wantErr)
			}
		})
	}
}

func (suite *UsersSuite) TestSetRole_WithoutStores() {
	uc := usecase.NewUsers(suite.log, usecase.UsersRepositories{Users: suite.usersRepo})
	suite.usersRepo.On("UpdateRole", mock.Anything, 1, models.RoleAdmin).Once().Return(nil)

	suite.NoError(uc.SetRole(context.Background(), 9, 1, models.RoleAdmin))
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
