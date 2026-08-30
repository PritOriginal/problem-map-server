package usecase_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/push"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/stretchr/testify/mock"
)

func (suite *NotificationsSuite) TestCreate() {
	android := models.UserDevice{ID: 1, UserID: 7, Platform: models.PlatformAndroid, Token: "tok-android"}
	web := models.UserDevice{ID: 2, UserID: 7, Platform: models.PlatformWeb, Token: "tok-web"}
	ios := models.UserDevice{ID: 3, UserID: 7, Platform: models.PlatformIOS, Token: "tok-ios"}

	errInvalid := fmt.Errorf("%w: fcm status 404 UNREGISTERED", push.ErrInvalidToken)
	errStub := fmt.Errorf("apns: %w", push.ErrNotImplemented)

	tests := []struct {
		name       string
		eventID    string
		add        method[int64]
		created    bool
		getDevices method[[]models.UserDevice]
		// sendErr is the Send outcome per token, deleteErr the DeleteDevice
		// outcome per token (only tokens rejected with ErrInvalidToken are
		// deleted), wantResults the recorded metric per token.
		sendErr     map[string]error
		deleteErr   map[string]error
		wantResults map[string]string
		wantID      int64
		wantCreated bool
		wantErr     bool
	}{
		{
			name:        "OkWithPush",
			eventID:     "evt-1",
			add:         method[int64]{data: 10},
			created:     true,
			getDevices:  method[[]models.UserDevice]{data: []models.UserDevice{android, web}},
			wantResults: map[string]string{android.Token: push.ResultOK, web.Token: push.ResultOK},
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
			getDevices:  method[[]models.UserDevice]{data: []models.UserDevice{android}},
			sendErr:     map[string]error{android.Token: errors.New("fcm down")},
			wantResults: map[string]string{android.Token: push.ResultError},
			wantID:      12,
			wantCreated: true,
		},
		{
			name:        "InvalidTokenDeletesDevice",
			eventID:     "evt-1",
			add:         method[int64]{data: 13},
			created:     true,
			getDevices:  method[[]models.UserDevice]{data: []models.UserDevice{android, web}},
			sendErr:     map[string]error{android.Token: errInvalid},
			deleteErr:   map[string]error{android.Token: nil},
			wantResults: map[string]string{android.Token: push.ResultInvalidToken, web.Token: push.ResultOK},
			wantID:      13,
			wantCreated: true,
		},
		{
			name:        "InvalidTokenAlreadyDeleted",
			eventID:     "evt-1",
			add:         method[int64]{data: 14},
			created:     true,
			getDevices:  method[[]models.UserDevice]{data: []models.UserDevice{web}},
			sendErr:     map[string]error{web.Token: errInvalid},
			deleteErr:   map[string]error{web.Token: repository.ErrNotFound},
			wantResults: map[string]string{web.Token: push.ResultInvalidToken},
			wantID:      14,
			wantCreated: true,
		},
		{
			name:        "InvalidTokenDeleteErrorIsSwallowed",
			eventID:     "evt-1",
			add:         method[int64]{data: 15},
			created:     true,
			getDevices:  method[[]models.UserDevice]{data: []models.UserDevice{android}},
			sendErr:     map[string]error{android.Token: errInvalid},
			deleteErr:   map[string]error{android.Token: errRepo},
			wantResults: map[string]string{android.Token: push.ResultInvalidToken},
			wantID:      15,
			wantCreated: true,
		},
		{
			name:        "UnsupportedPlatform",
			eventID:     "evt-1",
			add:         method[int64]{data: 16},
			created:     true,
			getDevices:  method[[]models.UserDevice]{data: []models.UserDevice{ios}},
			sendErr:     map[string]error{ios.Token: errStub},
			wantResults: map[string]string{ios.Token: push.ResultUnsupported},
			wantID:      16,
			wantCreated: true,
		},
		{
			name:        "DevicesErrorIsSwallowed",
			eventID:     "evt-1",
			add:         method[int64]{data: 17},
			created:     true,
			getDevices:  method[[]models.UserDevice]{err: errRepo},
			wantID:      17,
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
				for _, device := range tt.getDevices.data {
					suite.push.On("Send", mock.Anything, device, mock.MatchedBy(func(n models.Notification) bool {
						return n.ID == int(tt.add.data)
					})).Once().Return(tt.sendErr[device.Token])
					suite.metrics.On("PushSent", device.Platform, tt.wantResults[device.Token]).Once().Return()
					if deleteErr, ok := tt.deleteErr[device.Token]; ok {
						suite.devices.On("DeleteDevice", mock.Anything, 7, device.Token).Once().Return(deleteErr)
					}
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
