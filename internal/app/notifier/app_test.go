package notifier_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/app/notifier"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/nats"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/suite"
)

// recordingHandlers records every dispatched event.
type recordingHandlers struct {
	statusChanged []events.MarkStatusChanged
	taskAssigned  []events.TaskAssigned
	checkAdded    []events.CheckAdded
	assigned      []events.MarkAssigned
	slaBreached   []events.MarkSLABreached
	commentAdded  []events.CommentAdded
	err           error
}

func (h *recordingHandlers) HandleMarkAssigned(_ context.Context, ev events.MarkAssigned) error {
	h.assigned = append(h.assigned, ev)
	return h.err
}

func (h *recordingHandlers) HandleMarkSLABreached(_ context.Context, ev events.MarkSLABreached) error {
	h.slaBreached = append(h.slaBreached, ev)
	return h.err
}

func (h *recordingHandlers) HandleMarkStatusChanged(_ context.Context, ev events.MarkStatusChanged) error {
	h.statusChanged = append(h.statusChanged, ev)
	return h.err
}

func (h *recordingHandlers) HandleTaskAssigned(_ context.Context, ev events.TaskAssigned) error {
	h.taskAssigned = append(h.taskAssigned, ev)
	return h.err
}

func (h *recordingHandlers) HandleCommentAdded(_ context.Context, ev events.CommentAdded) error {
	h.commentAdded = append(h.commentAdded, ev)
	return h.err
}

func (h *recordingHandlers) HandleCheckAdded(_ context.Context, ev events.CheckAdded) error {
	h.checkAdded = append(h.checkAdded, ev)
	return h.err
}

type RouterSuite struct {
	suite.Suite
}

func TestRouter(t *testing.T) {
	suite.Run(t, new(RouterSuite))
}

func (suite *RouterSuite) TestHandle() {
	errHandler := errors.New("handler failed")

	statusEv := events.NewMarkStatusChanged(5, models.UnconfirmedStatus, models.ConfirmedStatus, 3)
	taskEv := events.NewTaskAssigned(9, 2, 5, nil)
	checkEv := events.NewCheckAdded(77, 5, 2)

	tests := []struct {
		name       string
		subject    string
		payload    any
		raw        []byte
		handlerErr error
		wantErr    error
		// rawOK marks a raw payload that must be handled without error.
		rawOK bool
		check func(h *recordingHandlers)
	}{
		{
			name: "MarkStatusChanged", subject: events.SubjectMarkStatusChanged, payload: statusEv,
			check: func(h *recordingHandlers) { suite.Equal([]events.MarkStatusChanged{statusEv}, h.statusChanged) },
		},
		{
			name: "TaskAssigned", subject: events.SubjectTaskAssigned, payload: taskEv,
			check: func(h *recordingHandlers) { suite.Equal([]events.TaskAssigned{taskEv}, h.taskAssigned) },
		},
		{
			name: "CheckAdded", subject: events.SubjectCheckAdded, payload: checkEv,
			check: func(h *recordingHandlers) { suite.Equal([]events.CheckAdded{checkEv}, h.checkAdded) },
		},
		{
			name: "HandlerErrorIsReturned", subject: events.SubjectCheckAdded, payload: checkEv,
			handlerErr: errHandler, wantErr: errHandler,
		},
		{
			name: "BadPayloadIsNotRetried", subject: events.SubjectCheckAdded, raw: []byte("{not json"),
			wantErr: nats.ErrNoRetry,
			check:   func(h *recordingHandlers) { suite.Empty(h.checkAdded) },
		},
		{
			name: "NewerSchemaVersion", subject: events.SubjectCheckAdded,
			raw:     []byte(`{"v":99,"event_id":"e1","check_id":1,"mark_id":5,"user_id":2}`),
			wantErr: events.ErrUnsupportedVersion,
			check:   func(h *recordingHandlers) { suite.Empty(h.checkAdded) },
		},
		{
			name: "NewerSchemaVersionIsNotRetried", subject: events.SubjectCheckAdded,
			raw:     []byte(`{"v":99,"event_id":"e1","check_id":1,"mark_id":5,"user_id":2}`),
			wantErr: nats.ErrNoRetry,
		},
		{
			name: "MissingVersionIsAccepted", subject: events.SubjectCheckAdded,
			raw:   []byte(`{"event_id":"e1","check_id":1,"mark_id":5,"user_id":2}`),
			rawOK: true,
			check: func(h *recordingHandlers) { suite.Len(h.checkAdded, 1) },
		},
		{
			name: "UnknownSubjectIsNotRetried", subject: "mark.deleted", raw: []byte("{}"),
			wantErr: nats.ErrNoRetry,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			h := &recordingHandlers{err: tt.handlerErr}
			router := notifier.NewRouter(slogdiscard.NewDiscardLogger(), h)

			data := tt.raw
			if tt.payload != nil {
				var err error
				data, err = json.Marshal(tt.payload)
				suite.Require().NoError(err)
			}

			err := router.Handle(context.Background(), tt.subject, data)
			switch {
			case tt.wantErr != nil:
				suite.ErrorIs(err, tt.wantErr)
			case tt.payload == nil && !tt.rawOK:
				suite.Error(err)
			default:
				suite.NoError(err)
			}
			if tt.check != nil {
				tt.check(h)
			}
		})
	}
}

// TestHandlerErrorIsRetried checks that a handler failure is not marked
// ErrNoRetry, so the consumer redelivers the event.
func (suite *RouterSuite) TestHandlerErrorIsRetried() {
	errHandler := errors.New("db down")
	data, err := json.Marshal(events.NewCheckAdded(77, 5, 2))
	suite.Require().NoError(err)

	err = notifier.NewRouter(slogdiscard.NewDiscardLogger(), &recordingHandlers{err: errHandler}).
		Handle(context.Background(), events.SubjectCheckAdded, data)
	suite.ErrorIs(err, errHandler)
	suite.NotErrorIs(err, nats.ErrNoRetry)
}

func (suite *RouterSuite) TestSubjects() {
	router := notifier.NewRouter(slogdiscard.NewDiscardLogger(), &recordingHandlers{})
	suite.ElementsMatch([]string{
		events.SubjectMarkStatusChanged, events.SubjectTaskAssigned, events.SubjectCheckAdded,
		events.SubjectMarkAssigned, events.SubjectMarkSLABreached, events.SubjectCommentAdded,
	}, router.Subjects())
}
