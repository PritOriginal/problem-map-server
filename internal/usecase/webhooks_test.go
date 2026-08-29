package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	"github.com/PritOriginal/problem-map-server/internal/usecase"
	"github.com/PritOriginal/problem-map-server/internal/webhooks"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/guregu/null/v6"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

var errWebhookBoom = errors.New("boom")

type WebhooksSuite struct {
	suite.Suite
	uc            *usecase.Webhooks
	repo          *usecase.MockWebhooksRepository
	sender        *usecase.MockWebhookSender
	urls          *usecase.MockWebhookURLValidator
	notifications *usecase.MockNotificationCreator
}

func (suite *WebhooksSuite) SetupTest() {
	suite.repo = usecase.NewMockWebhooksRepository(suite.T())
	suite.sender = usecase.NewMockWebhookSender(suite.T())
	suite.urls = usecase.NewMockWebhookURLValidator(suite.T())
	suite.notifications = usecase.NewMockNotificationCreator(suite.T())
	suite.uc = usecase.NewWebhooks(slogdiscard.NewDiscardLogger(), usecase.WebhooksDeps{
		Sender:        suite.sender,
		URLs:          suite.urls,
		Notifications: suite.notifications,
	}, usecase.WebhooksRepositories{Webhooks: suite.repo})
}

func TestWebhooks(t *testing.T) {
	suite.Run(t, new(WebhooksSuite))
}

var (
	owner = models.Actor{UserID: 7, Role: models.RoleModerator}
	admin = models.Actor{UserID: 1, Role: models.RoleAdmin}
	other = models.Actor{UserID: 9, Role: models.RoleModerator}
)

func testWebhook() models.Webhook {
	return models.Webhook{ID: 3, OwnerUserID: owner.UserID, URL: "https://example.org/hook", Secret: "s3cr3t", Events: []string{"mark.*"}, Active: true}
}

func (suite *WebhooksSuite) TestCreate() {
	tests := []struct {
		name       string
		in         models.Webhook
		urlErr     error
		addErr     error
		wantSecret string // "" means generated
		wantErr    error
	}{
		{name: "OkGeneratedSecret", in: models.Webhook{URL: "https://example.org/hook", Events: []string{"mark.status_changed"}}},
		{name: "OkOwnSecret", in: models.Webhook{URL: "https://example.org/hook", Events: []string{"*"}, Secret: "my-own-secret-123"}, wantSecret: "my-own-secret-123"},
		{name: "ErrForbiddenURL", in: models.Webhook{URL: "https://10.0.0.1/hook", Events: []string{"*"}}, urlErr: webhooks.ErrForbiddenTarget, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrUnknownEvent", in: models.Webhook{URL: "https://example.org/hook", Events: []string{"user.created"}}, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrNoEvents", in: models.Webhook{URL: "https://example.org/hook"}, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrRepo", in: models.Webhook{URL: "https://example.org/hook", Events: []string{"*"}}, addErr: errWebhookBoom, wantErr: errWebhookBoom},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.urls.On("Validate", mock.Anything, tt.in.URL).Once().Return(tt.urlErr)
			if tt.urlErr == nil && models.ValidateWebhookEvents(tt.in.Events, usecase.KnownWebhookEvents()) == nil {
				var stored models.Webhook
				suite.repo.On("AddWebhook", mock.Anything, mock.MatchedBy(func(w models.Webhook) bool {
					stored = w
					return w.OwnerUserID == owner.UserID && w.Active && w.Secret != ""
				})).Once().Return(int64(3), tt.addErr)
				if tt.addErr == nil {
					suite.repo.On("GetWebhookById", mock.Anything, 3).Once().Return(models.Webhook{ID: 3, OwnerUserID: owner.UserID, URL: tt.in.URL, Events: tt.in.Events, Active: true, Secret: "stored"}, nil)
				}
				defer func() {
					if tt.wantErr == nil {
						if tt.wantSecret != "" {
							suite.Equal(tt.wantSecret, stored.Secret)
						} else {
							suite.Len(stored.Secret, 64, "32 random bytes hex-encoded")
						}
					}
				}()
			}

			got, err := suite.uc.Create(context.Background(), owner, tt.in)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.Equal(3, got.ID)
			suite.NotEqual("stored", got.Secret, "the secret returned is the one generated/provided, not re-read")
			if tt.wantSecret != "" {
				suite.Equal(tt.wantSecret, got.Secret)
			}
		})
	}
}

func (suite *WebhooksSuite) TestUpdate() {
	active := false
	tests := []struct {
		name    string
		actor   models.Actor
		upd     models.WebhookUpdate
		getErr  error
		updErr  error
		wantErr error
	}{
		{name: "OkOwner", actor: owner, upd: models.WebhookUpdate{Active: &active}},
		{name: "OkAdmin", actor: admin, upd: models.WebhookUpdate{Events: []string{"check.added"}}},
		{name: "ErrForbidden", actor: other, upd: models.WebhookUpdate{Active: &active}, wantErr: usecase.ErrForbidden},
		{name: "ErrNotFound", actor: owner, upd: models.WebhookUpdate{Active: &active}, getErr: repository.ErrNotFound, wantErr: usecase.ErrNotFound},
		{name: "ErrEmpty", actor: owner, upd: models.WebhookUpdate{}, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrBadEvents", actor: owner, upd: models.WebhookUpdate{Events: []string{"nope"}}, wantErr: usecase.ErrInvalidArgument},
		{name: "ErrRepo", actor: owner, upd: models.WebhookUpdate{Active: &active}, updErr: errWebhookBoom, wantErr: errWebhookBoom},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			validInput := !tt.upd.IsEmpty() && (tt.upd.Events == nil || models.ValidateWebhookEvents(tt.upd.Events, usecase.KnownWebhookEvents()) == nil)
			if validInput {
				suite.repo.On("GetWebhookById", mock.Anything, 3).Once().Return(testWebhook(), tt.getErr)
			}
			if validInput && tt.getErr == nil && tt.actor != other {
				suite.repo.On("UpdateWebhook", mock.Anything, 3, tt.upd).Once().Return(tt.updErr)
				if tt.updErr == nil {
					updated := testWebhook()
					updated.Active = false
					suite.repo.On("GetWebhookById", mock.Anything, 3).Once().Return(updated, nil)
				}
			}

			got, err := suite.uc.Update(context.Background(), tt.actor, 3, tt.upd)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.Require().NoError(err)
			suite.False(got.Active)
		})
	}
}

func (suite *WebhooksSuite) TestDelete() {
	tests := []struct {
		name    string
		actor   models.Actor
		delErr  error
		wantErr error
	}{
		{name: "OkOwner", actor: owner},
		{name: "OkAdmin", actor: admin},
		{name: "ErrForbidden", actor: other, wantErr: usecase.ErrForbidden},
		{name: "ErrGone", actor: owner, delErr: repository.ErrNotFound, wantErr: usecase.ErrNotFound},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repo.On("GetWebhookById", mock.Anything, 3).Once().Return(testWebhook(), nil)
			if tt.actor != other {
				suite.repo.On("DeleteWebhook", mock.Anything, 3).Once().Return(tt.delErr)
			}

			err := suite.uc.Delete(context.Background(), tt.actor, 3)
			if tt.wantErr != nil {
				suite.ErrorIs(err, tt.wantErr)
				return
			}
			suite.NoError(err)
		})
	}
}

func (suite *WebhooksSuite) TestListDeliveries() {
	suite.Run("Ok", func() {
		p := models.Pagination{Limit: 10}
		suite.repo.On("GetWebhookById", mock.Anything, 3).Once().Return(testWebhook(), nil)
		suite.repo.On("GetDeliveriesByWebhookId", mock.Anything, 3, p).Once().
			Return(models.Page[models.WebhookDelivery]{Items: []models.WebhookDelivery{{ID: 1}}, Total: 1}, nil)

		page, err := suite.uc.ListDeliveries(context.Background(), owner, 3, p)
		suite.Require().NoError(err)
		suite.Equal(1, page.Total)
	})
	suite.Run("ErrPagination", func() {
		_, err := suite.uc.ListDeliveries(context.Background(), owner, 3, models.Pagination{Limit: models.MaxLimit + 1})
		suite.ErrorIs(err, usecase.ErrInvalidArgument)
	})
	suite.Run("ErrForbidden", func() {
		suite.repo.On("GetWebhookById", mock.Anything, 3).Once().Return(testWebhook(), nil)
		_, err := suite.uc.ListDeliveries(context.Background(), other, 3, models.Pagination{Limit: 10})
		suite.ErrorIs(err, usecase.ErrForbidden)
	})
}

func (suite *WebhooksSuite) TestSendTest() {
	tests := []struct {
		name   string
		result webhooks.Result
	}{
		{name: "Delivered", result: webhooks.Result{StatusCode: 200}},
		{name: "FailedNoRetry", result: webhooks.Result{StatusCode: 503, Err: errors.New("unexpected status 503")}},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			w := testWebhook()
			suite.repo.On("GetWebhookById", mock.Anything, 3).Once().Return(w, nil)
			suite.repo.On("AddDelivery", mock.Anything, mock.MatchedBy(func(d models.WebhookDelivery) bool {
				var payload models.WebhookPayload
				suite.Require().NoError(json.Unmarshal(d.Payload, &payload))
				return d.WebhookID == 3 && d.Subject == models.WebhookSubjectTest && payload.EventID == d.EventID && payload.Subject == models.WebhookSubjectTest
			})).Once().Return(func(_ context.Context, d models.WebhookDelivery) (models.WebhookDelivery, bool, error) {
				d.ID = 11
				return d, true, nil
			})
			suite.sender.On("Send", mock.Anything, mock.MatchedBy(func(r webhooks.Request) bool {
				return r.WebhookID == 3 && r.URL == w.URL && r.Secret == w.Secret
			})).Once().Return(tt.result)
			suite.repo.On("RecordAttempt", mock.Anything, int64(11), mock.MatchedBy(func(r models.WebhookAttemptResult) bool {
				return r.Attempt == 1 && r.DeliveredAt.Valid == tt.result.OK() && !r.NextAttemptAt.Valid
			})).Once().Return(nil)

			d, err := suite.uc.SendTest(context.Background(), owner, 3)
			suite.Require().NoError(err)
			suite.Equal(1, d.Attempt)
			suite.Equal(int64(tt.result.StatusCode), d.StatusCode.ValueOrZero())
			suite.Equal(tt.result.OK(), d.Delivered())
			suite.False(d.NextAttemptAt.Valid, "test deliveries are never retried")
		})
	}
}

func (suite *WebhooksSuite) TestDispatch() {
	data := json.RawMessage(`{"v":1,"event_id":"e1","mark_id":5}`)

	suite.Run("NoSubscribers", func() {
		suite.repo.On("GetActiveWebhooksForEvent", mock.Anything, events.SubjectMarkStatusChanged).Once().Return(nil, nil)
		suite.NoError(suite.uc.Dispatch(context.Background(), events.SubjectMarkStatusChanged, "e1", data))
	})

	suite.Run("FanOutAndSkipDuplicates", func() {
		hookOK := testWebhook()
		hookDup := testWebhook()
		hookDup.ID = 4
		hookFail := testWebhook()
		hookFail.ID = 5
		suite.repo.On("GetActiveWebhooksForEvent", mock.Anything, events.SubjectMarkStatusChanged).Once().
			Return([]models.Webhook{hookOK, hookDup, hookFail}, nil)

		addDelivery := func(webhookID int, created bool) {
			suite.repo.On("AddDelivery", mock.Anything, mock.MatchedBy(func(d models.WebhookDelivery) bool {
				var payload models.WebhookPayload
				suite.Require().NoError(json.Unmarshal(d.Payload, &payload))
				return d.WebhookID == webhookID && d.EventID == "e1" && d.Subject == events.SubjectMarkStatusChanged &&
					d.NextAttemptAt.Valid && payload.EventID == "e1" && string(payload.Data) == string(data)
			})).Once().Return(func(_ context.Context, d models.WebhookDelivery) (models.WebhookDelivery, bool, error) {
				d.ID = int64(100 + webhookID)
				return d, created, nil
			})
		}
		addDelivery(3, true)
		addDelivery(4, false)
		addDelivery(5, true)

		suite.sender.On("Send", mock.Anything, mock.MatchedBy(func(r webhooks.Request) bool { return r.WebhookID == 3 })).Once().
			Return(webhooks.Result{StatusCode: 200})
		suite.sender.On("Send", mock.Anything, mock.MatchedBy(func(r webhooks.Request) bool { return r.WebhookID == 5 })).Once().
			Return(webhooks.Result{Err: errors.New("dial tcp: connection refused")})
		suite.repo.On("RecordAttempt", mock.Anything, int64(103), mock.MatchedBy(func(r models.WebhookAttemptResult) bool {
			return r.Attempt == 1 && r.DeliveredAt.Valid && r.StatusCode.ValueOrZero() == 200
		})).Once().Return(nil)
		suite.repo.On("RecordAttempt", mock.Anything, int64(105), mock.MatchedBy(func(r models.WebhookAttemptResult) bool {
			return r.Attempt == 1 && !r.DeliveredAt.Valid && !r.StatusCode.Valid && r.Error.Valid && r.NextAttemptAt.Valid
		})).Once().Return(nil)

		suite.NoError(suite.uc.Dispatch(context.Background(), events.SubjectMarkStatusChanged, "e1", data))
	})

	suite.Run("RecordErrorIsReported", func() {
		suite.repo.On("GetActiveWebhooksForEvent", mock.Anything, events.SubjectCheckAdded).Once().Return([]models.Webhook{testWebhook()}, nil)
		suite.repo.On("AddDelivery", mock.Anything, mock.Anything).Once().Return(models.WebhookDelivery{ID: 50, WebhookID: 3, EventID: "e2"}, true, nil)
		suite.sender.On("Send", mock.Anything, mock.Anything).Once().Return(webhooks.Result{StatusCode: 204})
		suite.repo.On("RecordAttempt", mock.Anything, int64(50), mock.Anything).Once().Return(errWebhookBoom)

		suite.ErrorIs(suite.uc.Dispatch(context.Background(), events.SubjectCheckAdded, "e2", data), errWebhookBoom)
	})
}

func (suite *WebhooksSuite) TestRetryDue() {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		attempt       int // attempts already made
		result        webhooks.Result
		wantDelivered bool
		wantNext      time.Time
		wantDisabled  bool
	}{
		{name: "SecondAttemptSucceeds", attempt: 1, result: webhooks.Result{StatusCode: 200}, wantDelivered: true},
		{name: "SecondAttemptFailsSchedulesThird", attempt: 1, result: webhooks.Result{StatusCode: 500, Err: errors.New("500")}, wantNext: now.Add(5 * time.Minute)},
		{name: "FifthAttemptFailsSchedulesSixth", attempt: 4, result: webhooks.Result{Err: errors.New("timeout")}, wantNext: now.Add(12 * time.Hour)},
		{name: "LastAttemptFailsDisables", attempt: models.MaxWebhookAttempts - 1, result: webhooks.Result{Err: errors.New("timeout")}, wantDisabled: true},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			w := testWebhook()
			pending := models.PendingWebhookDelivery{
				Delivery: models.WebhookDelivery{ID: 21, WebhookID: w.ID, EventID: "e1", Subject: events.SubjectTaskAssigned, Payload: json.RawMessage(`{}`), Attempt: tt.attempt},
				Webhook:  w,
			}
			suite.repo.On("ClaimDueDeliveries", mock.Anything, mock.Anything, mock.Anything, 50).Once().Return([]models.PendingWebhookDelivery{pending}, nil)
			suite.sender.On("Send", mock.Anything, mock.MatchedBy(func(r webhooks.Request) bool { return r.EventID == "e1" && r.Secret == w.Secret })).Once().Return(tt.result)
			suite.repo.On("RecordAttempt", mock.Anything, int64(21), mock.MatchedBy(func(r models.WebhookAttemptResult) bool {
				if r.Attempt != tt.attempt+1 || r.DeliveredAt.Valid != tt.wantDelivered {
					return false
				}
				if tt.wantNext.IsZero() {
					return !r.NextAttemptAt.Valid
				}
				// The use case stamps time.Now; only the delay is checked.
				return r.NextAttemptAt.Valid && time.Until(r.NextAttemptAt.Time).Round(time.Minute) == tt.wantNext.Sub(now)
			})).Once().Return(nil)
			if tt.wantDisabled {
				suite.repo.On("UpdateWebhook", mock.Anything, w.ID, mock.MatchedBy(func(u models.WebhookUpdate) bool {
					return u.Active != nil && !*u.Active && u.Events == nil
				})).Once().Return(nil)
				suite.notifications.On("Create", mock.Anything, mock.MatchedBy(func(n models.Notification) bool {
					return n.UserID == w.OwnerUserID && n.Type == models.NotificationWebhookDisabled
				})).Once().Return(int64(1), true, nil)
			}

			n, err := suite.uc.RetryDue(context.Background(), 50)
			suite.NoError(err)
			suite.Equal(1, n)
		})
	}

	suite.Run("ClaimError", func() {
		suite.repo.On("ClaimDueDeliveries", mock.Anything, mock.Anything, mock.Anything, 5).Once().Return(nil, errWebhookBoom)
		_, err := suite.uc.RetryDue(context.Background(), 5)
		suite.ErrorIs(err, errWebhookBoom)
	})
}

// TestDeliveryThroughHTTPServer drives a Dispatch through the real
// webhooks.Sender to an httptest receiver and checks the wire format.
func (suite *WebhooksSuite) TestDeliveryThroughHTTPServer() {
	type seen struct {
		headers http.Header
		body    []byte
	}
	var (
		mu       sync.Mutex
		requests []seen
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, seen{headers: r.Header.Clone(), body: body})
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	sender := webhooks.NewSender(webhooks.SenderOptions{
		Timeout:   2 * time.Second,
		Policy:    webhooks.URLPolicy{AllowPrivate: true},
		Transport: srv.Client().Transport,
	})
	uc := usecase.NewWebhooks(slogdiscard.NewDiscardLogger(), usecase.WebhooksDeps{Sender: sender, URLs: suite.urls},
		usecase.WebhooksRepositories{Webhooks: suite.repo})

	w := testWebhook()
	w.URL = srv.URL + "/hook"
	ev := events.NewCheckAdded(77, 5, 2)
	data, err := json.Marshal(ev)
	suite.Require().NoError(err)

	suite.repo.On("GetActiveWebhooksForEvent", mock.Anything, events.SubjectCheckAdded).Once().Return([]models.Webhook{w}, nil)
	suite.repo.On("AddDelivery", mock.Anything, mock.Anything).Once().Return(func(_ context.Context, d models.WebhookDelivery) (models.WebhookDelivery, bool, error) {
		d.ID = 1
		return d, true, nil
	})
	suite.repo.On("RecordAttempt", mock.Anything, int64(1), mock.MatchedBy(func(r models.WebhookAttemptResult) bool {
		return r.DeliveredAt.Valid && r.StatusCode.ValueOrZero() == http.StatusAccepted
	})).Once().Return(nil)

	suite.Require().NoError(uc.Dispatch(context.Background(), events.SubjectCheckAdded, ev.EventID, data))

	suite.Require().Len(requests, 1)
	got := requests[0]
	suite.Equal("3", got.headers.Get(webhooks.HeaderWebhookID))
	suite.Equal(ev.EventID, got.headers.Get(webhooks.HeaderEventID))
	suite.NotEmpty(got.headers.Get(webhooks.HeaderTimestamp))
	suite.True(webhooks.VerifySignature(w.Secret, got.body, got.headers.Get(webhooks.HeaderSignature)))

	var payload models.WebhookPayload
	suite.Require().NoError(json.Unmarshal(got.body, &payload))
	suite.Equal(ev.EventID, payload.EventID)
	suite.Equal(events.SubjectCheckAdded, payload.Subject)
	suite.WithinDuration(time.Now(), payload.OccurredAt, time.Minute)
	suite.JSONEq(string(data), string(payload.Data))
}

// Keep null imported for the attempt-result matchers above.
var _ = null.IntFrom
