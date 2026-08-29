package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SyncSuite struct {
	suite.Suite
	uc            *usecase.Sync
	tasks         *usecase.MockSyncTasksRepository
	notifications *usecase.MockSyncNotificationsRepository
	checks        *usecase.MockSyncChecksRepository
}

func (suite *SyncSuite) SetupTest() {
	suite.tasks = usecase.NewMockSyncTasksRepository(suite.T())
	suite.notifications = usecase.NewMockSyncNotificationsRepository(suite.T())
	suite.checks = usecase.NewMockSyncChecksRepository(suite.T())
	suite.uc = usecase.NewSync(slogdiscard.NewDiscardLogger(), usecase.SyncRepositories{
		Tasks:         suite.tasks,
		Notifications: suite.notifications,
		Checks:        suite.checks,
	})
}

func TestSync(t *testing.T) {
	suite.Run(t, new(SyncSuite))
}

func (suite *SyncSuite) TestGetUserSync() {
	const userID = 7
	since := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	p := models.Pagination{Limit: 50, Offset: 10}

	tests := []struct {
		name          string
		filters       models.UserSyncFilters
		tasks         method[models.Page[models.Task]]
		notifications method[models.Page[models.Notification]]
		checks        method[models.Page[models.Check]]
		wantErrArg    bool
	}{
		{
			name:          "Ok",
			filters:       models.UserSyncFilters{Since: since, Pagination: p},
			tasks:         method[models.Page[models.Task]]{data: models.Page[models.Task]{Items: []models.Task{{ID: 1}}, Total: 3}},
			notifications: method[models.Page[models.Notification]]{data: models.Page[models.Notification]{Items: []models.Notification{{ID: 2}, {ID: 3}}, Total: 2}},
			checks:        method[models.Page[models.Check]]{data: models.Page[models.Check]{Items: []models.Check{}, Total: 0}},
		},
		{name: "ErrSinceRequired", filters: models.UserSyncFilters{Pagination: p}, wantErrArg: true},
		{name: "ErrPagination", filters: models.UserSyncFilters{Since: since, Pagination: models.Pagination{Limit: 501}}, wantErrArg: true},
		{
			name:    "ErrTasks",
			filters: models.UserSyncFilters{Since: since, Pagination: p},
			tasks:   method[models.Page[models.Task]]{err: errRepo},
		},
		{
			name:          "ErrNotifications",
			filters:       models.UserSyncFilters{Since: since, Pagination: p},
			tasks:         method[models.Page[models.Task]]{},
			notifications: method[models.Page[models.Notification]]{err: errRepo},
		},
		{
			name:          "ErrChecks",
			filters:       models.UserSyncFilters{Since: since, Pagination: p},
			tasks:         method[models.Page[models.Task]]{},
			notifications: method[models.Page[models.Notification]]{},
			checks:        method[models.Page[models.Check]]{err: errRepo},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if !tt.wantErrArg {
				suite.tasks.On("GetTasksByUserId", mock.Anything, userID, models.GetTasksByUserIdFilters{
					UpdatedSince: since, Pagination: tt.filters.Pagination,
				}).Once().Return(tt.tasks.data, tt.tasks.err)
				if tt.tasks.err == nil {
					suite.notifications.On("GetNotificationsByUserId", mock.Anything, userID, models.GetNotificationsFilters{
						UnreadOnly: true, CreatedSince: since, Pagination: tt.filters.Pagination,
					}).Once().Return(tt.notifications.data, tt.notifications.err)
				}
				if tt.tasks.err == nil && tt.notifications.err == nil {
					suite.checks.On("GetChecksByUserIdSince", mock.Anything, userID, since, tt.filters.Pagination).
						Once().Return(tt.checks.data, tt.checks.err)
				}
			}

			before := time.Now().UTC()
			got, err := suite.uc.GetUserSync(context.Background(), userID, tt.filters)

			switch {
			case tt.wantErrArg:
				suite.ErrorIs(err, usecase.ErrInvalidArgument)
			case tt.tasks.err != nil:
				assertRepoErr(&suite.Suite, err, tt.tasks.err)
			case tt.notifications.err != nil:
				assertRepoErr(&suite.Suite, err, tt.notifications.err)
			case tt.checks.err != nil:
				assertRepoErr(&suite.Suite, err, tt.checks.err)
			default:
				suite.Require().NoError(err)
				suite.Equal(tt.tasks.data.Items, got.Tasks)
				suite.Equal(tt.notifications.data.Items, got.Notifications)
				suite.Equal(tt.checks.data.Items, got.Checks)
				suite.Equal(models.UserSyncTotals{
					Tasks: tt.tasks.data.Total, Notifications: tt.notifications.data.Total, Checks: tt.checks.data.Total,
				}, got.Totals)
				suite.False(got.ServerTime.Before(before.Truncate(time.Second)))
				suite.Equal(time.UTC, got.ServerTime.Location())
			}
		})
	}
}
