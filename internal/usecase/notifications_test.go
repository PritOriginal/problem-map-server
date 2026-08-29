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

type NotificationsSuite struct {
	suite.Suite
	uc      *usecase.Notifications
	log     *slog.Logger
	repo    *usecase.MockNotificationsRepository
	devices *usecase.MockDevicesRepository
	push    *usecase.MockPushSender
}

func (suite *NotificationsSuite) SetupTest() {
	suite.log = slogdiscard.NewDiscardLogger()
	suite.repo = usecase.NewMockNotificationsRepository(suite.T())
	suite.devices = usecase.NewMockDevicesRepository(suite.T())
	suite.push = usecase.NewMockPushSender(suite.T())
	suite.uc = usecase.NewNotifications(suite.log, suite.push, usecase.NotificationsRepositories{
		Notifications: suite.repo,
		Devices:       suite.devices,
	})
}

func TestNotifications(t *testing.T) {
	suite.Run(t, new(NotificationsSuite))
}

func (suite *NotificationsSuite) TestCreate() {
	devices := []models.UserDevice{{ID: 1, UserID: 7, Platform: models.PlatformAndroid, Token: "tok"}}

	tests := []struct {
		name        string
		eventID     string
		add         method[int64]
		created     bool
		getDevices  method[[]models.UserDevice]
		pushErr     error
		wantID      int64
		wantCreated bool
		wantErr     bool
	}{
		{
			name:        "OkWithPush",
			eventID:     "evt-1",
			add:         method[int64]{data: 10},
			created:     true,
			getDevices:  method[[]models.UserDevice]{data: devices},
			wantID:      10,
			wantCreated: true,
		},
		{
			name:        "OkGeneratesEventID",
			add:         method[int64]{data: 11},
			created:     true,
			getDevices:  method[[]models.UserDevice]{data: nil},
			wantID:      11,
			wantCreated: true,
		},
		{
			name:        "DuplicateSkipped",
			eventID:     "evt-1",
			add:         method[int64]{data: 0},
			created:     false,
			wantID:      0,
			wantCreated: false,
		},
		{
			name:        "PushErrorIsSwallowed",
			eventID:     "evt-1",
			add:         method[int64]{data: 12},
			created:     true,
			getDevices:  method[[]models.UserDevice]{data: devices},
			pushErr:     errors.New("fcm down"),
			wantID:      12,
			wantCreated: true,
		},
		{
			name:        "DevicesErrorIsSwallowed",
			eventID:     "evt-1",
			add:         method[int64]{data: 13},
			created:     true,
			getDevices:  method[[]models.UserDevice]{err: errRepo},
			wantID:      13,
			wantCreated: true,
		},
		{
			name:    "ErrRepo",
			eventID: "evt-1",
			add:     method[int64]{err: errRepo},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repo.On("AddNotification", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
				if tt.eventID != "" {
					return n.EventID == tt.eventID
				}
				return n.EventID != ""
			})).Once().Return(tt.add.data, tt.created, tt.add.err)

			if tt.add.err == nil && tt.created {
				suite.devices.On("GetDevicesByUserId", mock.Anything, 7).Once().Return(tt.getDevices.data, tt.getDevices.err)
				if tt.getDevices.err == nil && len(tt.getDevices.data) > 0 {
					suite.push.On("Send", mock.Anything, tt.getDevices.data, mock.MatchedBy(func(n models.Notification) bool {
						return n.ID == int(tt.add.data)
					})).Once().Return(tt.pushErr)
				}
			}

			id, created, err := suite.uc.Create(context.Background(), models.Notification{
				UserID:  7,
				EventID: tt.eventID,
				Type:    models.NotificationTaskAssigned,
				Title:   "t",
			})

			if tt.wantErr {
				assertRepoErr(&suite.Suite, err, tt.add.err)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.wantID, id)
			suite.Equal(tt.wantCreated, created)
		})
	}
}

func (suite *NotificationsSuite) TestCreateWithoutPushSender() {
	uc := usecase.NewNotifications(suite.log, nil, usecase.NotificationsRepositories{
		Notifications: suite.repo,
		Devices:       suite.devices,
	})
	suite.repo.On("AddNotification", mock.Anything, mock.Anything).Once().Return(int64(1), true, nil)

	id, created, err := uc.Create(context.Background(), models.Notification{UserID: 1, EventID: "e"})
	suite.NoError(err)
	suite.Equal(int64(1), id)
	suite.True(created)
}

func (suite *NotificationsSuite) TestList() {
	tests := []struct {
		name    string
		filters models.GetNotificationsFilters
		list    method[models.Page[models.Notification]]
		wantErr error
	}{
		{
			name:    "Ok",
			filters: models.GetNotificationsFilters{UnreadOnly: true, Pagination: models.Pagination{Limit: 10}},
			list:    method[models.Page[models.Notification]]{data: models.Page[models.Notification]{Items: []models.Notification{{ID: 1}}, Total: 1}},
		},
		{
			name:    "ErrInvalidPagination",
			filters: models.GetNotificationsFilters{Pagination: models.Pagination{Limit: -1}},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name: "ErrRepo",
			list: method[models.Page[models.Notification]]{err: errRepo},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantErr == nil {
				suite.repo.On("GetNotificationsByUserId", mock.Anything, 7, tt.filters).Once().Return(tt.list.data, tt.list.err)
			}

			page, err := suite.uc.List(context.Background(), 7, tt.filters)

			switch {
			case tt.wantErr != nil:
				suite.ErrorIs(err, tt.wantErr)
			case tt.list.err != nil:
				assertRepoErr(&suite.Suite, err, tt.list.err)
			default:
				suite.NoError(err)
				suite.Equal(tt.list.data, page)
			}
		})
	}
}

func (suite *NotificationsSuite) TestUnreadCount() {
	tests := []struct {
		name  string
		count method[int]
	}{
		{name: "Ok", count: method[int]{data: 3}},
		{name: "ErrRepo", count: method[int]{err: errRepo}},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repo.On("CountUnreadByUserId", mock.Anything, 7).Once().Return(tt.count.data, tt.count.err)

			got, err := suite.uc.UnreadCount(context.Background(), 7)
			if tt.count.err != nil {
				assertRepoErr(&suite.Suite, err, tt.count.err)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.count.data, got)
		})
	}
}

func (suite *NotificationsSuite) TestMarkRead() {
	tests := []struct {
		name    string
		repoErr error
	}{
		{name: "Ok"},
		{name: "ErrNotFound", repoErr: repository.ErrNotFound},
		{name: "ErrRepo", repoErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repo.On("MarkRead", mock.Anything, 7, 42).Once().Return(tt.repoErr)

			err := suite.uc.MarkRead(context.Background(), 7, 42)
			if tt.repoErr != nil {
				assertRepoErr(&suite.Suite, err, tt.repoErr)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *NotificationsSuite) TestMarkAllRead() {
	tests := []struct {
		name    string
		updated method[int64]
	}{
		{name: "Ok", updated: method[int64]{data: 5}},
		{name: "ErrRepo", updated: method[int64]{err: errRepo}},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repo.On("MarkAllRead", mock.Anything, 7).Once().Return(tt.updated.data, tt.updated.err)

			got, err := suite.uc.MarkAllRead(context.Background(), 7)
			if tt.updated.err != nil {
				assertRepoErr(&suite.Suite, err, tt.updated.err)
				return
			}
			suite.NoError(err)
			suite.Equal(tt.updated.data, got)
		})
	}
}

func (suite *NotificationsSuite) TestRegisterDevice() {
	tests := []struct {
		name    string
		device  models.UserDevice
		upsert  method[models.UserDevice]
		wantErr error
	}{
		{
			name:   "Ok",
			device: models.UserDevice{UserID: 7, Platform: models.PlatformIOS, Token: "tok"},
			upsert: method[models.UserDevice]{data: models.UserDevice{ID: 1, UserID: 7, Platform: models.PlatformIOS, Token: "tok"}},
		},
		{
			name:    "ErrUnknownPlatform",
			device:  models.UserDevice{UserID: 7, Platform: "windows", Token: "tok"},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:    "ErrEmptyToken",
			device:  models.UserDevice{UserID: 7, Platform: models.PlatformWeb},
			wantErr: usecase.ErrInvalidArgument,
		},
		{
			name:   "ErrRepo",
			device: models.UserDevice{UserID: 7, Platform: models.PlatformAndroid, Token: "tok"},
			upsert: method[models.UserDevice]{err: errRepo},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.wantErr == nil {
				suite.devices.On("UpsertDevice", mock.Anything, tt.device).Once().Return(tt.upsert.data, tt.upsert.err)
			}

			got, err := suite.uc.RegisterDevice(context.Background(), tt.device)
			switch {
			case tt.wantErr != nil:
				suite.ErrorIs(err, tt.wantErr)
			case tt.upsert.err != nil:
				assertRepoErr(&suite.Suite, err, tt.upsert.err)
			default:
				suite.NoError(err)
				suite.Equal(tt.upsert.data, got)
			}
		})
	}
}

func (suite *NotificationsSuite) TestDeleteDevice() {
	tests := []struct {
		name    string
		repoErr error
	}{
		{name: "Ok"},
		{name: "ErrNotFound", repoErr: repository.ErrNotFound},
		{name: "ErrRepo", repoErr: errRepo},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.devices.On("DeleteDevice", mock.Anything, 7, "tok").Once().Return(tt.repoErr)

			err := suite.uc.DeleteDevice(context.Background(), 7, "tok")
			if tt.repoErr != nil {
				assertRepoErr(&suite.Suite, err, tt.repoErr)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *NotificationsSuite) TestLogPushSender() {
	sender := usecase.NewLogPushSender(suite.log)
	suite.NoError(sender.Send(context.Background(), []models.UserDevice{{Platform: models.PlatformWeb, Token: "t"}}, models.Notification{UserID: 1}))
}
