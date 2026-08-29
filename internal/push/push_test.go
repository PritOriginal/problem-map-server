package push_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/push"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"
)

// recorder is a Sender that records the platform of every device it got.
type recorder struct {
	platforms []models.DevicePlatform
	err       error
}

func (r *recorder) Send(_ context.Context, device models.UserDevice, _ models.Notification) error {
	r.platforms = append(r.platforms, device.Platform)
	return r.err
}

type PushSuite struct {
	suite.Suite
}

func TestPush(t *testing.T) {
	suite.Run(t, new(PushSuite))
}

func (suite *PushSuite) TestData() {
	tests := []struct {
		name string
		n    models.Notification
		want map[string]string
	}{
		{
			name: "AllIDs",
			n:    models.Notification{ID: 5, Type: models.NotificationTaskAssigned, MarkID: null.IntFrom(3), TaskID: null.IntFrom(9)},
			want: map[string]string{"type": string(models.NotificationTaskAssigned), "notification_id": "5", "mark_id": "3", "task_id": "9"},
		},
		{
			name: "NoOptionalIDs",
			n:    models.Notification{ID: 6, Type: models.NotificationTaskAssigned},
			want: map[string]string{"type": string(models.NotificationTaskAssigned), "notification_id": "6"},
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.Equal(tt.want, push.Data(tt.n))
		})
	}
}

func (suite *PushSuite) TestMulti() {
	errFCM := errors.New("fcm failed")

	tests := []struct {
		name     string
		platform models.DevicePlatform
		fallback bool
		wantFCM  bool
		wantLog  bool
		wantErr  error
	}{
		{name: "AndroidToFCM", platform: models.PlatformAndroid, fallback: true, wantFCM: true, wantErr: errFCM},
		{name: "WebToFCM", platform: models.PlatformWeb, fallback: true, wantFCM: true, wantErr: errFCM},
		{name: "IOSToFallback", platform: models.PlatformIOS, fallback: true, wantLog: true},
		{name: "IOSWithoutFallback", platform: models.PlatformIOS, wantErr: push.ErrNotImplemented},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			fcm := &recorder{err: errFCM}
			fallback := &recorder{}
			var fb push.Sender
			if tt.fallback {
				fb = fallback
			}
			m := push.NewMulti(fb, map[models.DevicePlatform]push.Sender{
				models.PlatformAndroid: fcm,
				models.PlatformWeb:     fcm,
				models.PlatformIOS:     nil,
			})

			err := m.Send(context.Background(), models.UserDevice{Platform: tt.platform, Token: "t"}, models.Notification{})

			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
			} else {
				suite.NoError(err)
			}
			suite.Equal(tt.wantFCM, len(fcm.platforms) == 1)
			suite.Equal(tt.wantLog, len(fallback.platforms) == 1)
		})
	}
}

func (suite *PushSuite) TestLogSender() {
	sender := push.NewLogSender(slogdiscard.NewDiscardLogger())
	suite.NoError(sender.Send(context.Background(), models.UserDevice{Platform: models.PlatformWeb, Token: "t"}, models.Notification{UserID: 1}))
}

func (suite *PushSuite) TestMetrics() {
	reg := prometheus.NewRegistry()
	m := push.NewMetrics(reg)

	m.PushSent(models.PlatformAndroid, push.ResultOK)
	m.PushSent(models.PlatformAndroid, push.ResultOK)
	m.PushSent(models.PlatformIOS, push.ResultUnsupported)

	expected := `
# HELP push_sent_total Push notifications sent, by platform and result (ok, invalid_token, error, unsupported).
# TYPE push_sent_total counter
push_sent_total{platform="android",result="ok"} 2
push_sent_total{platform="ios",result="unsupported"} 1
`
	suite.NoError(testutil.GatherAndCompare(reg, strings.NewReader(expected), "push_sent_total"))
}
