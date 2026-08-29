package notifier_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/app/notifier"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/suite"
)

type dispatched struct {
	subject string
	eventID string
	data    json.RawMessage
}

// recordingDispatcher records Dispatch calls and counts RetryDue calls.
type recordingDispatcher struct {
	mu         sync.Mutex
	dispatched []dispatched
	retries    int
	prunes     int
	err        error
	retryErr   error
}

func (d *recordingDispatcher) PruneDeliveries(context.Context) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.prunes++
	return 1, nil
}

func (d *recordingDispatcher) pruneCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.prunes
}

func (d *recordingDispatcher) Dispatch(_ context.Context, subject, eventID string, data json.RawMessage) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dispatched = append(d.dispatched, dispatched{subject: subject, eventID: eventID, data: data})
	return d.err
}

func (d *recordingDispatcher) RetryDue(context.Context, int) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.retries++
	return 1, d.retryErr
}

func (d *recordingDispatcher) retryCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.retries
}

// recordingSubjectSubscriber records the wildcard subscriptions.
type recordingSubjectSubscriber struct {
	patterns []string
	queues   []string
	err      error
}

func (s *recordingSubjectSubscriber) QueueSubscribeSubject(pattern, queue string, _ notifier.SubjectHandler) (*natsgo.Subscription, error) {
	s.patterns = append(s.patterns, pattern)
	s.queues = append(s.queues, queue)
	return nil, s.err
}

type WebhookRouterSuite struct {
	suite.Suite
}

func TestWebhookRouter(t *testing.T) {
	suite.Run(t, new(WebhookRouterSuite))
}

func (suite *WebhookRouterSuite) TestHandle() {
	errDispatch := errors.New("dispatch failed")
	statusEv := events.NewMarkStatusChanged(5, models.UnconfirmedStatus, models.ConfirmedStatus, 3)
	statusRaw, err := json.Marshal(statusEv)
	suite.Require().NoError(err)

	tests := []struct {
		name        string
		subject     string
		raw         []byte
		dispatchErr error
		wantErr     error
		wantNone    bool
		check       func(d dispatched)
	}{
		{
			name: "PassesPayloadThrough", subject: events.SubjectMarkStatusChanged, raw: statusRaw,
			check: func(d dispatched) {
				suite.Equal(events.SubjectMarkStatusChanged, d.subject)
				suite.Equal(statusEv.EventID, d.eventID)
				suite.JSONEq(string(statusRaw), string(d.data))
			},
		},
		{
			name: "UnknownFutureSubjectIsForwarded", subject: "mark.deleted",
			raw:   []byte(`{"v":1,"event_id":"e-del","mark_id":7}`),
			check: func(d dispatched) { suite.Equal("e-del", d.eventID) },
		},
		{
			name: "MissingEventIDIsGenerated", subject: events.SubjectCheckAdded,
			raw:   []byte(`{"check_id":1,"mark_id":5,"user_id":2}`),
			check: func(d dispatched) { suite.Len(d.eventID, 36) },
		},
		{
			name: "DispatchErrorIsReturned", subject: events.SubjectCheckAdded, raw: statusRaw,
			dispatchErr: errDispatch, wantErr: errDispatch,
		},
		{name: "BadPayload", subject: events.SubjectCheckAdded, raw: []byte("{not json"), wantNone: true},
		{
			name: "NewerSchemaVersion", subject: events.SubjectCheckAdded,
			raw: []byte(`{"v":99,"event_id":"e1"}`), wantErr: events.ErrUnsupportedVersion, wantNone: true,
		},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			d := &recordingDispatcher{err: tt.dispatchErr}
			router := notifier.NewWebhookRouter(slogdiscard.NewDiscardLogger(), d)

			err := router.Handle(context.Background(), tt.subject, tt.raw)
			switch {
			case tt.wantErr != nil:
				suite.ErrorIs(err, tt.wantErr)
			case tt.wantNone:
				suite.Error(err)
			default:
				suite.NoError(err)
			}
			if tt.wantNone {
				suite.Empty(d.dispatched)
				return
			}
			suite.Require().Len(d.dispatched, 1)
			if tt.check != nil {
				tt.check(d.dispatched[0])
			}
		})
	}
}

func (suite *WebhookRouterSuite) TestSubscribe() {
	sub := &recordingSubjectSubscriber{}
	router := notifier.NewWebhookRouter(slogdiscard.NewDiscardLogger(), &recordingDispatcher{})

	suite.Require().NoError(router.Subscribe(sub))
	suite.Equal(notifier.WebhookSubjects, sub.patterns)
	for _, q := range sub.queues {
		suite.Equal(notifier.WebhookQueueGroup, q)
	}
	suite.NotEqual(notifier.QueueGroup, notifier.WebhookQueueGroup, "both consumers must receive every event")

	failing := &recordingSubjectSubscriber{err: errors.New("nats down")}
	suite.Error(router.Subscribe(failing))
}

func (suite *WebhookRouterSuite) TestRetryLoop() {
	d := &recordingDispatcher{retryErr: errors.New("one failed")}
	router := notifier.NewWebhookRouter(slogdiscard.NewDiscardLogger(), d)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		router.RetryLoop(ctx, 5*time.Millisecond, 10)
	}()

	suite.Eventually(func() bool { return d.retryCount() >= 3 }, 2*time.Second, time.Millisecond)
	suite.Equal(1, d.pruneCount(), "the log is pruned once at start, then every PruneInterval")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		suite.Fail("RetryLoop did not stop on context cancellation")
	}
}
