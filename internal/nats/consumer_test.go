package nats

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

// ConsumerSuite tests the ack decision of Consumer.handle with a mocked
// JetStream message: ack on success, nak with backoff on a failure,
// dead-letter (copy to the DLQ + term) after MaxDeliver or on ErrNoRetry.
type ConsumerSuite struct {
	suite.Suite
}

func TestConsumer(t *testing.T) {
	suite.Run(t, new(ConsumerSuite))
}

func (s *ConsumerSuite) newConsumer(handler MsgHandler, dlq DLQPublisher, m *Metrics) *Consumer {
	return &Consumer{
		log:     slogdiscard.NewDiscardLogger(),
		metrics: m,
		cfg: ConsumerConfig{
			Name:       "notifier",
			Subjects:   []string{"check.added"},
			MaxDeliver: 3,
			Backoff:    []time.Duration{time.Second, 5 * time.Second},
		}.withDefaults(),
		handler: handler,
		dlq:     dlq,
	}
}

func (s *ConsumerSuite) newMsg(delivered uint64) *MockMsg {
	msg := NewMockMsg(s.T())
	msg.EXPECT().Subject().Return("check.added").Maybe()
	msg.EXPECT().Data().Return([]byte(`{"event_id":"e1"}`)).Maybe()
	msg.EXPECT().Headers().Return(nats.Header{jetstream.MsgIDHeader: []string{"e1"}}).Maybe()
	msg.EXPECT().Metadata().Return(&jetstream.MsgMetadata{
		NumDelivered: delivered,
		Stream:       StreamEvents,
		Consumer:     "notifier",
		Sequence:     jetstream.SequencePair{Stream: 42, Consumer: 7},
	}, nil).Maybe()
	return msg
}

func (s *ConsumerSuite) TestHandle() {
	errHandler := errors.New("db down")
	errDLQ := errors.New("dlq unavailable")

	tests := []struct {
		name       string
		delivered  uint64
		handlerErr error
		panics     bool
		dlqErr     error
		expect     func(msg *MockMsg, dlq *MockDLQPublisher)
		// consumed is the expected result label of events_consumed_total.
		consumed     string
		redeliveries float64
	}{
		{
			name: "AckOnSuccess", delivered: 1,
			expect:   func(msg *MockMsg, _ *MockDLQPublisher) { msg.EXPECT().Ack().Return(nil).Once() },
			consumed: ResultAck,
		},
		{
			name: "AckFailureIsLogged", delivered: 2,
			expect:       func(msg *MockMsg, _ *MockDLQPublisher) { msg.EXPECT().Ack().Return(nats.ErrConnectionClosed).Once() },
			consumed:     ResultAck,
			redeliveries: 1,
		},
		{
			name: "NakWithFirstBackoff", delivered: 1, handlerErr: errHandler,
			expect:   func(msg *MockMsg, _ *MockDLQPublisher) { msg.EXPECT().NakWithDelay(time.Second).Return(nil).Once() },
			consumed: ResultNak,
		},
		{
			name: "NakWithLastBackoffRepeated", delivered: 2, handlerErr: errHandler,
			expect: func(msg *MockMsg, _ *MockDLQPublisher) {
				msg.EXPECT().NakWithDelay(5 * time.Second).Return(nil).Once()
			},
			consumed:     ResultNak,
			redeliveries: 1,
		},
		{
			name: "DeadLetterAfterMaxDeliver", delivered: 3, handlerErr: errHandler,
			expect: func(msg *MockMsg, dlq *MockDLQPublisher) {
				dlq.EXPECT().PublishMsg(mock.Anything, mock.MatchedBy(func(m *nats.Msg) bool {
					return m.Subject == "dlq.check.added" &&
						string(m.Data) == `{"event_id":"e1"}` &&
						m.Header.Get(HeaderDLQSubject) == "check.added" &&
						m.Header.Get(HeaderDLQDeliveries) == "3" &&
						m.Header.Get(HeaderDLQStreamSeq) == "42" &&
						m.Header.Get(HeaderDLQConsumer) == "notifier" &&
						m.Header.Get(HeaderDLQError) == errHandler.Error() &&
						m.Header.Get(HeaderDLQMsgID) == "e1"
				}), mock.Anything).Return(&jetstream.PubAck{Stream: StreamDLQ, Sequence: 1}, nil).Once()
				msg.EXPECT().Term().Return(nil).Once()
			},
			consumed:     ResultDLQ,
			redeliveries: 1,
		},
		{
			name: "DeadLetterImmediatelyOnNoRetry", delivered: 1,
			handlerErr: fmt.Errorf("%w: decode failed", ErrNoRetry),
			expect: func(msg *MockMsg, dlq *MockDLQPublisher) {
				dlq.EXPECT().PublishMsg(mock.Anything, mock.Anything, mock.Anything).
					Return(&jetstream.PubAck{Stream: StreamDLQ, Sequence: 2}, nil).Once()
				msg.EXPECT().Term().Return(nil).Once()
			},
			consumed: ResultDLQ,
		},
		{
			name: "PanicIsDeadLettered", delivered: 1, panics: true,
			expect: func(msg *MockMsg, dlq *MockDLQPublisher) {
				dlq.EXPECT().PublishMsg(mock.Anything, mock.Anything, mock.Anything).
					Return(&jetstream.PubAck{Stream: StreamDLQ, Sequence: 3}, nil).Once()
				msg.EXPECT().Term().Return(nil).Once()
			},
			consumed: ResultDLQ,
		},
		{
			name: "NakWhenDeadLetterFails", delivered: 3, handlerErr: errHandler, dlqErr: errDLQ,
			expect: func(msg *MockMsg, dlq *MockDLQPublisher) {
				dlq.EXPECT().PublishMsg(mock.Anything, mock.Anything, mock.Anything).Return(nil, errDLQ).Once()
				msg.EXPECT().Nak().Return(nil).Once()
			},
			consumed:     ResultError,
			redeliveries: 1,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			reg := prometheus.NewRegistry()
			m := NewMetrics(reg)

			var calls int
			handler := func(_ context.Context, subject string, data []byte) error {
				calls++
				s.Equal("check.added", subject)
				s.JSONEq(`{"event_id":"e1"}`, string(data))
				if tt.panics {
					panic("boom")
				}
				return tt.handlerErr
			}

			msg := s.newMsg(tt.delivered)
			dlq := NewMockDLQPublisher(s.T())
			tt.expect(msg, dlq)

			s.newConsumer(handler, dlq, m).handle(msg)

			s.Equal(1, calls)
			s.Equal(float64(1), testutil.ToFloat64(m.consumed.WithLabelValues("check.added", tt.consumed)))
			s.Equal(tt.redeliveries, testutil.ToFloat64(m.redeliveries))
		})
	}
}

// TestHandleWithoutMetadata checks that a message whose metadata cannot be
// parsed is treated as a first delivery.
func (s *ConsumerSuite) TestHandleWithoutMetadata() {
	msg := NewMockMsg(s.T())
	msg.EXPECT().Subject().Return("check.added").Maybe()
	msg.EXPECT().Data().Return([]byte(`{}`)).Maybe()
	msg.EXPECT().Metadata().Return(nil, jetstream.ErrNotJSMessage).Maybe()
	msg.EXPECT().NakWithDelay(time.Second).Return(nil).Once()

	handler := func(context.Context, string, []byte) error { return errors.New("fail") }
	s.newConsumer(handler, NewMockDLQPublisher(s.T()), nil).handle(msg)
}

func (s *ConsumerSuite) TestBackoff() {
	cfg := ConsumerConfig{Backoff: []time.Duration{time.Second, 5 * time.Second, 30 * time.Second}}
	s.Equal(time.Second, cfg.backoff(0))
	s.Equal(time.Second, cfg.backoff(1))
	s.Equal(5*time.Second, cfg.backoff(2))
	s.Equal(30*time.Second, cfg.backoff(3))
	s.Equal(30*time.Second, cfg.backoff(99))
}

func (s *ConsumerSuite) TestWithDefaults() {
	cfg := ConsumerConfig{Name: "n"}.withDefaults()
	s.Equal(DefaultMaxDeliver, cfg.MaxDeliver)
	s.Equal(DefaultBackoff, cfg.Backoff)
	s.Equal(defaultAckWait, cfg.AckWait)

	custom := ConsumerConfig{MaxDeliver: 2, Backoff: []time.Duration{time.Millisecond}, AckWait: time.Minute}.withDefaults()
	s.Equal(2, custom.MaxDeliver)
	s.Equal([]time.Duration{time.Millisecond}, custom.Backoff)
	s.Equal(time.Minute, custom.AckWait)
}

// TestNilMetrics checks that a consumer without a registry does not panic.
func (s *ConsumerSuite) TestNilMetrics() {
	var m *Metrics
	m.recordPublished("a", ResultOK)
	m.recordConsumed("a", ResultAck)
	m.recordRedelivery()
}
