package events_test

import (
	"context"
	"errors"
	"testing"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type EventsSuite struct {
	suite.Suite
}

func TestEvents(t *testing.T) {
	suite.Run(t, new(EventsSuite))
}

func (suite *EventsSuite) TestConstructorsSetEventID() {
	suite.NotEmpty(events.NewMarkStatusChanged(1, models.UnconfirmedStatus, models.ConfirmedStatus, 2).EventID)
	suite.NotEmpty(events.NewTaskAssigned(1, 2, 3, nil).EventID)
	suite.NotEmpty(events.NewCheckAdded(1, 2, 3).EventID)
	suite.Equal(events.SubjectMarkStatusChanged, events.MarkStatusChanged{}.Subject())
	suite.Equal(events.SubjectTaskAssigned, events.TaskAssigned{}.Subject())
	suite.Equal(events.SubjectCheckAdded, events.CheckAdded{}.Subject())
}

func (suite *EventsSuite) TestPublishEvent() {
	tests := []struct {
		name string
		err  error
	}{
		{name: "Ok"},
		{name: "ErrIsLoggedNotReturned", err: errors.New("broker down")},
	}
	for _, tt := range tests {
		suite.Run(tt.name, func() {
			pub := events.NewMockPublisher(suite.T())
			ev := events.NewCheckAdded(1, 2, 3)
			pub.On("Publish", mock.Anything, events.SubjectCheckAdded, ev).Once().Return(tt.err)

			suite.NotPanics(func() {
				events.PublishEvent(context.Background(), slogdiscard.NewDiscardLogger(), pub, ev)
			})
		})
	}
	suite.NotPanics(func() {
		events.PublishEvent(context.Background(), slogdiscard.NewDiscardLogger(), nil, events.CheckAdded{})
	})
	suite.NoError(events.NoopPublisher{}.Publish(context.Background(), "x", nil))
}

func (suite *EventsSuite) TestPending() {
	ctx := context.Background()
	suite.False(events.Collect(ctx, events.CheckAdded{}), "no pending in ctx")

	var pending events.Pending
	ctx = events.WithPending(ctx, &pending)
	ev1 := events.NewCheckAdded(1, 2, 3)
	ev2 := events.NewMarkStatusChanged(2, models.UnconfirmedStatus, models.ConfirmedStatus, 4)
	suite.True(events.Collect(ctx, ev1))
	suite.True(events.Collect(ctx, ev2))
	suite.Equal([]events.Event{ev1, ev2}, pending.Events())

	pub := events.NewMockPublisher(suite.T())
	pub.On("Publish", mock.Anything, events.SubjectCheckAdded, ev1).Once().Return(nil)
	pub.On("Publish", mock.Anything, events.SubjectMarkStatusChanged, ev2).Once().Return(nil)
	pending.Flush(ctx, slogdiscard.NewDiscardLogger(), pub)
	suite.Empty(pending.Events())
}
