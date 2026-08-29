package fcm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/push"
	"github.com/PritOriginal/problem-map-server/internal/push/fcm"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/suite"
	"golang.org/x/oauth2"
)

// fcmError builds the Google API error envelope with an FcmError detail.
func fcmError(status int, code, message string) []byte {
	body := map[string]any{"error": map[string]any{
		"code": status, "message": message, "status": "ERROR",
		"details": []map[string]any{{"@type": "type.googleapis.com/google.firebase.fcm.v1.FcmError", "errorCode": code}},
	}}
	raw, _ := json.Marshal(body)
	return raw
}

// response is one scripted answer of the fake FCM server.
type response struct {
	status     int
	body       []byte
	retryAfter string
}

// fakeFCM answers requests from a script, one response per request (the
// last one repeats), and records what it received.
type fakeFCM struct {
	mu       sync.Mutex
	script   []response
	requests []*http.Request
	bodies   []map[string]any
	inFlight atomic.Int32
	maxSeen  atomic.Int32
}

func (f *fakeFCM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		seen := f.maxSeen.Load()
		if n <= seen || f.maxSeen.CompareAndSwap(seen, n) {
			break
		}
	}
	// Let concurrent requests overlap so the concurrency limit is observable.
	time.Sleep(20 * time.Millisecond)

	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	f.mu.Lock()
	f.requests = append(f.requests, r)
	f.bodies = append(f.bodies, body)
	i := min(len(f.requests)-1, len(f.script)-1)
	resp := f.script[i]
	f.mu.Unlock()

	if resp.retryAfter != "" {
		w.Header().Set("Retry-After", resp.retryAfter)
	}
	w.WriteHeader(resp.status)
	_, _ = w.Write(resp.body)
}

func (f *fakeFCM) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

type FCMSuite struct {
	suite.Suite
}

func TestFCM(t *testing.T) {
	suite.Run(t, new(FCMSuite))
}

func (suite *FCMSuite) newSender(server *httptest.Server, cfg config.FCMConfig) *fcm.Sender {
	if cfg.Timeout == 0 {
		cfg.Timeout = time.Second
	}
	if cfg.Concurrency == 0 {
		cfg.Concurrency = 8
	}
	if cfg.ProjectID == "" {
		cfg.ProjectID = "test-project"
	}
	sender, err := fcm.New(slogdiscard.NewDiscardLogger(), cfg,
		fcm.WithBaseURL(server.URL),
		fcm.WithHTTPClient(server.Client()),
		fcm.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "secret-token"})),
		fcm.WithBackoff(time.Millisecond),
	)
	suite.Require().NoError(err)
	return sender
}

func (suite *FCMSuite) TestSend() {
	device := models.UserDevice{UserID: 7, Platform: models.PlatformAndroid, Token: "tok"}

	tests := []struct {
		name         string
		script       []response
		maxRetries   int
		wantErr      error
		wantAnyErr   bool
		wantRequests int
	}{
		{
			name:         "Ok",
			script:       []response{{status: http.StatusOK, body: []byte(`{"name":"projects/p/messages/1"}`)}},
			maxRetries:   3,
			wantRequests: 1,
		},
		{
			name: "ServerErrorThenOk",
			script: []response{
				{status: http.StatusInternalServerError, body: fcmError(500, "INTERNAL", "boom")},
				{status: http.StatusServiceUnavailable, body: fcmError(503, "UNAVAILABLE", "later")},
				{status: http.StatusOK, body: []byte(`{}`)},
			},
			maxRetries:   3,
			wantRequests: 3,
		},
		{
			name:         "ServerErrorExhaustsRetries",
			script:       []response{{status: http.StatusInternalServerError, body: fcmError(500, "INTERNAL", "boom")}},
			maxRetries:   3,
			wantAnyErr:   true,
			wantRequests: 4,
		},
		{
			name: "TooManyRequestsRetried",
			script: []response{
				{status: http.StatusTooManyRequests, body: fcmError(429, "QUOTA_EXCEEDED", "slow down"), retryAfter: "0"},
				{status: http.StatusOK, body: []byte(`{}`)},
			},
			maxRetries:   3,
			wantRequests: 2,
		},
		{
			name:         "NoRetriesConfigured",
			script:       []response{{status: http.StatusBadGateway, body: nil}},
			maxRetries:   0,
			wantAnyErr:   true,
			wantRequests: 1,
		},
		{
			name:         "Unregistered",
			script:       []response{{status: http.StatusNotFound, body: fcmError(404, "UNREGISTERED", "Requested entity was not found.")}},
			maxRetries:   3,
			wantErr:      push.ErrInvalidToken,
			wantRequests: 1,
		},
		{
			name:         "InvalidArgument",
			script:       []response{{status: http.StatusBadRequest, body: fcmError(400, "INVALID_ARGUMENT", "The registration token is not a valid FCM registration token")}},
			maxRetries:   3,
			wantErr:      push.ErrInvalidToken,
			wantRequests: 1,
		},
		{
			name:         "OtherClientErrorNotRetried",
			script:       []response{{status: http.StatusForbidden, body: fcmError(403, "SENDER_ID_MISMATCH", "mismatch")}},
			maxRetries:   3,
			wantAnyErr:   true,
			wantRequests: 1,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			fake := &fakeFCM{script: tt.script}
			server := httptest.NewServer(fake)
			defer server.Close()

			sender := suite.newSender(server, config.FCMConfig{MaxRetries: tt.maxRetries})
			err := sender.Send(context.Background(), device, models.Notification{ID: 1, Type: models.NotificationTaskAssigned, Title: "t"})

			switch {
			case tt.wantErr != nil:
				suite.ErrorIs(err, tt.wantErr)
			case tt.wantAnyErr:
				suite.Error(err)
				suite.NotErrorIs(err, push.ErrInvalidToken)
			default:
				suite.NoError(err)
			}
			suite.Equal(tt.wantRequests, fake.count())
		})
	}
}

func (suite *FCMSuite) TestPayload() {
	n := models.Notification{
		ID: 42, UserID: 7, Type: models.NotificationTaskAssigned,
		MarkID: null.IntFrom(3), TaskID: null.IntFrom(9), Title: "Title", Body: "Body",
	}
	wantData := map[string]any{"type": string(models.NotificationTaskAssigned), "notification_id": "42", "mark_id": "3", "task_id": "9"}

	tests := []struct {
		name        string
		platform    models.DevicePlatform
		wantSection string
	}{
		{name: "Android", platform: models.PlatformAndroid, wantSection: "android"},
		{name: "Web", platform: models.PlatformWeb, wantSection: "webpush"},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			fake := &fakeFCM{script: []response{{status: http.StatusOK, body: []byte(`{}`)}}}
			server := httptest.NewServer(fake)
			defer server.Close()

			sender := suite.newSender(server, config.FCMConfig{ProjectID: "my-project"})
			suite.Require().NoError(sender.Send(context.Background(), models.UserDevice{Platform: tt.platform, Token: "tok"}, n))

			suite.Require().Len(fake.requests, 1)
			req := fake.requests[0]
			suite.Equal("/v1/projects/my-project/messages:send", req.URL.Path)
			suite.Equal("Bearer secret-token", req.Header.Get("Authorization"))
			suite.Contains(req.Header.Get("Content-Type"), "application/json")

			msg, ok := fake.bodies[0]["message"].(map[string]any)
			suite.Require().True(ok, "message section")
			suite.Equal("tok", msg["token"])
			suite.Equal(map[string]any{"title": "Title", "body": "Body"}, msg["notification"])
			suite.Equal(wantData, msg["data"])
			suite.Contains(msg, tt.wantSection)
			if tt.platform == models.PlatformWeb {
				suite.NotContains(msg, "android")
				webpush, _ := msg["webpush"].(map[string]any)
				suite.Equal(map[string]any{"title": "Title", "body": "Body"}, webpush["notification"])
			}
		})
	}
}

func (suite *FCMSuite) TestConcurrencyLimit() {
	fake := &fakeFCM{script: []response{{status: http.StatusOK, body: []byte(`{}`)}}}
	server := httptest.NewServer(fake)
	defer server.Close()

	const limit = 2
	sender := suite.newSender(server, config.FCMConfig{Concurrency: limit})

	var wg sync.WaitGroup
	for i := range 6 {
		wg.Go(func() {
			suite.NoError(sender.Send(context.Background(), models.UserDevice{Platform: models.PlatformAndroid, Token: "tok"}, models.Notification{ID: i}))
		})
	}
	wg.Wait()

	suite.Equal(6, fake.count())
	suite.LessOrEqual(fake.maxSeen.Load(), int32(limit))
}

func (suite *FCMSuite) TestContextCancelledStopsRetries() {
	fake := &fakeFCM{script: []response{{status: http.StatusInternalServerError, body: nil}}}
	server := httptest.NewServer(fake)
	defer server.Close()

	sender := suite.newSender(server, config.FCMConfig{MaxRetries: 3})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sender.Send(ctx, models.UserDevice{Platform: models.PlatformAndroid, Token: "tok"}, models.Notification{})
	suite.ErrorIs(err, context.Canceled)
	suite.EqualError(err, "fcm: context canceled")
}

func (suite *FCMSuite) TestNew() {
	const serviceAccount = `{
		"type": "service_account",
		"project_id": "sa-project",
		"private_key_id": "k1",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIBVQIBADANBgkqhkiG9w0BAQEFAASCAT8wggE7AgEAAkEAtest\n-----END PRIVATE KEY-----\n",
		"client_email": "notifier@sa-project.iam.gserviceaccount.com",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`

	tests := []struct {
		name    string
		cfg     config.FCMConfig
		wantErr string
	}{
		{
			name: "CredentialsJSON",
			cfg:  config.FCMConfig{CredentialsJSON: serviceAccount, Timeout: time.Second, MaxRetries: 1, Concurrency: 1},
		},
		{
			name:    "NoCredentials",
			cfg:     config.FCMConfig{Timeout: time.Second, MaxRetries: 1, Concurrency: 1},
			wantErr: "credentials are not configured",
		},
		{
			name:    "MissingFile",
			cfg:     config.FCMConfig{CredentialsFile: "/nonexistent/sa.json", Timeout: time.Second, MaxRetries: 1, Concurrency: 1},
			wantErr: "read credentials file",
		},
		{
			name:    "BadJSON",
			cfg:     config.FCMConfig{CredentialsJSON: "{not json", Timeout: time.Second, MaxRetries: 1, Concurrency: 1},
			wantErr: "parse service account",
		},
		{
			name:    "NoProjectID",
			cfg:     config.FCMConfig{CredentialsJSON: `{"type":"service_account","client_email":"a@b","private_key":"x"}`, Timeout: time.Second, MaxRetries: 1, Concurrency: 1},
			wantErr: "project id",
		},
		{
			name:    "InvalidConfig",
			cfg:     config.FCMConfig{CredentialsJSON: serviceAccount, Timeout: 0, MaxRetries: 1, Concurrency: 1},
			wantErr: "FCM_TIMEOUT",
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			sender, err := fcm.New(slogdiscard.NewDiscardLogger(), tt.cfg)
			if tt.wantErr != "" {
				suite.ErrorContains(err, tt.wantErr)
				suite.Nil(sender)
				return
			}
			suite.NoError(err)
			suite.NotNil(sender)
		})
	}
}

func (suite *FCMSuite) TestTokenSourceError() {
	fake := &fakeFCM{script: []response{{status: http.StatusOK, body: []byte(`{}`)}}}
	server := httptest.NewServer(fake)
	defer server.Close()

	errToken := errors.New("no token")
	sender, err := fcm.New(slogdiscard.NewDiscardLogger(),
		config.FCMConfig{ProjectID: "p", Timeout: time.Second, MaxRetries: 1, Concurrency: 1},
		fcm.WithBaseURL(server.URL),
		fcm.WithTokenSource(failingTokenSource{err: errToken}),
		fcm.WithBackoff(time.Millisecond),
	)
	suite.Require().NoError(err)

	err = sender.Send(context.Background(), models.UserDevice{Platform: models.PlatformAndroid, Token: "tok"}, models.Notification{})
	suite.ErrorIs(err, errToken)
	suite.Equal(0, fake.count())
}

type failingTokenSource struct{ err error }

func (f failingTokenSource) Token() (*oauth2.Token, error) { return nil, f.err }
