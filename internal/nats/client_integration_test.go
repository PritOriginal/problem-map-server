//go:build integration

package nats_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/nats"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const natsImage = "nats:2-alpine"

// startNATS starts a NATS container (with JetStream when js is set) and
// returns the client URL.
func startNATS(s *suite.Suite, ctx context.Context, js bool) string {
	var cmd []string
	if js {
		cmd = []string{"-js"}
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        natsImage,
			Cmd:          cmd,
			ExposedPorts: []string{"4222/tcp"},
			WaitingFor:   wait.ForLog("Server is ready").WithStartupTimeout(time.Minute),
		},
		Started: true,
	})
	s.Require().NoError(err, "start nats container")
	testcontainers.CleanupContainer(s.T(), container)

	endpoint, err := container.PortEndpoint(ctx, "4222/tcp", "nats")
	s.Require().NoError(err)
	return endpoint
}

// NatsSuite runs the client against a real NATS server with JetStream.
// The streams are shared by the tests, so every test uses its own consumer
// name and its own subject under check.> (a durable consumer with
// DeliverAll would otherwise see the events of the other tests).
type NatsSuite struct {
	suite.Suite

	ctx context.Context
	cfg config.NatsConfig
}

func TestNatsSuite(t *testing.T) {
	suite.Run(t, new(NatsSuite))
}

func (s *NatsSuite) SetupSuite() {
	s.ctx = context.Background()
	s.cfg = config.NatsConfig{URL: startNATS(&s.Suite, s.ctx, true), Name: "test"}
}

func (s *NatsSuite) newClient(m *nats.Metrics) *nats.Client {
	client, err := nats.New(slogdiscard.NewDiscardLogger(), s.cfg, nats.WithMetrics(m))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = client.Close() })
	s.Require().True(client.JetStream(), "server must have jetstream enabled")
	return client
}

// received collects the deliveries of a consumer.
type received struct {
	mu    sync.Mutex
	items []events.CheckAdded
	ch    chan events.CheckAdded
}

func newReceived() *received { return &received{ch: make(chan events.CheckAdded, 16)} }

func (r *received) add(ev events.CheckAdded) {
	r.mu.Lock()
	r.items = append(r.items, ev)
	r.mu.Unlock()
	r.ch <- ev
}

func (r *received) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

func (r *received) wait(s *NatsSuite, timeout time.Duration) events.CheckAdded {
	select {
	case ev := <-r.ch:
		return ev
	case <-time.After(timeout):
		s.Require().Fail("event not received")
		return events.CheckAdded{}
	}
}

func (r *received) none(s *NatsSuite, d time.Duration) {
	select {
	case ev := <-r.ch:
		s.Failf("unexpected event", "%+v", ev)
	case <-time.After(d):
	}
}

// subject returns a test-private subject captured by StreamEvents.
func subject(name string) string { return "check." + name }

func decode(data []byte) (events.CheckAdded, error) {
	var ev events.CheckAdded
	err := json.Unmarshal(data, &ev)
	return ev, err
}

// TestPublishConsumeAck: an event published before the consumer exists is
// delivered (persistence), acked and not delivered again, even to a fresh
// consumer instance with the same durable name.
func (s *NatsSuite) TestPublishConsumeAck() {
	pubMetrics := nats.NewMetrics(prometheus.NewRegistry())
	publisher := s.newClient(pubMetrics)

	want := events.NewCheckAdded(77, 5, 2)
	var pub events.Publisher = publisher
	s.Require().NoError(pub.Publish(s.ctx, subject("ack-test"), want))
	s.Equal(float64(1), testutil.ToFloat64(pubMetrics.Published().WithLabelValues(subject("ack-test"), nats.ResultOK)))

	consMetrics := nats.NewMetrics(prometheus.NewRegistry())
	subscriber := s.newClient(consMetrics)
	got := newReceived()
	consumer, err := subscriber.Consume(s.ctx, nats.ConsumerConfig{
		Name: "ack-test", Subjects: []string{subject("ack-test")},
	}, func(_ context.Context, subj string, data []byte) error {
		s.Equal(subject("ack-test"), subj)
		ev, err := decode(data)
		if err != nil {
			return err
		}
		got.add(ev)
		return nil
	})
	s.Require().NoError(err)

	s.Equal(want, got.wait(s, 5*time.Second))
	s.Eventually(func() bool {
		return testutil.ToFloat64(consMetrics.Consumed().WithLabelValues(subject("ack-test"), nats.ResultAck)) == 1
	}, 5*time.Second, 50*time.Millisecond)

	// Another subject of the stream is not delivered to this consumer.
	s.Require().NoError(pub.Publish(s.ctx, events.SubjectMarkStatusChanged,
		events.NewMarkStatusChanged(1, models.UnconfirmedStatus, models.ConfirmedStatus, 1)))
	got.none(s, 300*time.Millisecond)

	// Restarting the consumer does not replay the acked event.
	s.Require().NoError(consumer.Stop(s.ctx))
	_, err = subscriber.Consume(s.ctx, nats.ConsumerConfig{
		Name: "ack-test", Subjects: []string{subject("ack-test")},
	}, func(_ context.Context, _ string, data []byte) error {
		ev, err := decode(data)
		if err != nil {
			return err
		}
		got.add(ev)
		return nil
	})
	s.Require().NoError(err)
	got.none(s, 500*time.Millisecond)
	s.Equal(1, got.count())
}

// TestRedeliveryAfterNak: a failed handler gets the event again after the
// backoff; the second attempt succeeds and acks.
func (s *NatsSuite) TestRedeliveryAfterNak() {
	m := nats.NewMetrics(prometheus.NewRegistry())
	client := s.newClient(m)

	got := newReceived()
	var attempts int
	_, err := client.Consume(s.ctx, nats.ConsumerConfig{
		Name:     "nak-test",
		Subjects: []string{subject("nak-test")},
		Backoff:  []time.Duration{200 * time.Millisecond},
	}, func(_ context.Context, _ string, data []byte) error {
		ev, err := decode(data)
		if err != nil {
			return err
		}
		if ev.CheckID != 1 {
			return nil // events of other tests
		}
		attempts++
		if attempts == 1 {
			return errors.New("transient failure")
		}
		got.add(ev)
		return nil
	})
	s.Require().NoError(err)

	want := events.NewCheckAdded(1, 5, 2)
	s.Require().NoError(client.Publish(s.ctx, subject("nak-test"), want))

	s.Equal(want, got.wait(s, 5*time.Second))
	s.Equal(2, attempts)
	s.Eventually(func() bool { return testutil.ToFloat64(m.Redeliveries()) >= 1 }, 5*time.Second, 50*time.Millisecond)
	s.Equal(float64(1), testutil.ToFloat64(m.Consumed().WithLabelValues(subject("nak-test"), nats.ResultNak)))
}

// TestDedupByMsgID: publishing the same event (same event_id) twice inside
// the duplicates window stores it once.
func (s *NatsSuite) TestDedupByMsgID() {
	m := nats.NewMetrics(prometheus.NewRegistry())
	client := s.newClient(m)

	got := newReceived()
	_, err := client.Consume(s.ctx, nats.ConsumerConfig{
		Name: "dedup-test", Subjects: []string{subject("dedup-test")},
	}, func(_ context.Context, _ string, data []byte) error {
		ev, err := decode(data)
		if err != nil {
			return err
		}
		if ev.CheckID == 2 {
			got.add(ev)
		}
		return nil
	})
	s.Require().NoError(err)

	want := events.NewCheckAdded(2, 5, 2)
	s.Require().NoError(client.Publish(s.ctx, subject("dedup-test"), want))
	s.Require().NoError(client.Publish(s.ctx, subject("dedup-test"), want))
	s.Equal(float64(1), testutil.ToFloat64(m.Published().WithLabelValues(subject("dedup-test"), nats.ResultOK)))
	s.Equal(float64(1), testutil.ToFloat64(m.Published().WithLabelValues(subject("dedup-test"), nats.ResultDuplicate)))

	s.Equal(want, got.wait(s, 5*time.Second))
	got.none(s, 500*time.Millisecond)
}

// TestDeadLetterAfterMaxDeliver: an event that keeps failing is moved to
// the DLQ stream after MaxDeliver attempts and is not delivered again.
func (s *NatsSuite) TestDeadLetterAfterMaxDeliver() {
	m := nats.NewMetrics(prometheus.NewRegistry())
	client := s.newClient(m)

	const maxDeliver = 3
	attempts := make(chan struct{}, 16)
	_, err := client.Consume(s.ctx, nats.ConsumerConfig{
		Name:       "dlq-test",
		Subjects:   []string{subject("dlq-test")},
		MaxDeliver: maxDeliver,
		Backoff:    []time.Duration{100 * time.Millisecond},
	}, func(_ context.Context, _ string, data []byte) error {
		ev, err := decode(data)
		if err != nil {
			return err
		}
		if ev.CheckID != 3 {
			return nil
		}
		attempts <- struct{}{}
		return errors.New("poison")
	})
	s.Require().NoError(err)

	want := events.NewCheckAdded(3, 5, 2)
	s.Require().NoError(client.Publish(s.ctx, subject("dlq-test"), want))

	for range maxDeliver {
		select {
		case <-attempts:
		case <-time.After(5 * time.Second):
			s.Require().Fail("delivery attempt missing")
		}
	}
	s.Eventually(func() bool {
		return testutil.ToFloat64(m.Consumed().WithLabelValues(subject("dlq-test"), nats.ResultDLQ)) == 1
	}, 5*time.Second, 50*time.Millisecond)
	select {
	case <-attempts:
		s.Fail("delivered after max deliver")
	case <-time.After(500 * time.Millisecond):
	}

	// The copy in the DLQ carries the payload and the diagnostic headers.
	js := s.jetStream(client)
	dlq, err := js.Stream(s.ctx, nats.StreamDLQ)
	s.Require().NoError(err)
	info, err := dlq.Info(s.ctx)
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(info.State.Msgs, uint64(1))

	msg, err := dlq.GetLastMsgForSubject(s.ctx, nats.DLQSubjectPrefix+subject("dlq-test"))
	s.Require().NoError(err)
	gotEv, err := decode(msg.Data)
	s.Require().NoError(err)
	s.Equal(want, gotEv)
	s.Equal(subject("dlq-test"), msg.Header.Get(nats.HeaderDLQSubject))
	s.Equal("dlq-test", msg.Header.Get(nats.HeaderDLQConsumer))
	s.Equal("3", msg.Header.Get(nats.HeaderDLQDeliveries))
	s.Equal(nats.StreamEvents, msg.Header.Get(nats.HeaderDLQStream))
	s.Contains(msg.Header.Get(nats.HeaderDLQError), "poison")
	s.Equal(want.EventID, msg.Header.Get(nats.HeaderDLQMsgID))
	s.Equal("dlq-dlq-test-"+want.EventID, msg.Header.Get(jetstream.MsgIDHeader))
}

// TestNoRetryGoesStraightToDLQ: a handler error wrapped in ErrNoRetry is
// dead-lettered on the first delivery.
func (s *NatsSuite) TestNoRetryGoesStraightToDLQ() {
	m := nats.NewMetrics(prometheus.NewRegistry())
	client := s.newClient(m)

	attempts := make(chan struct{}, 16)
	_, err := client.Consume(s.ctx, nats.ConsumerConfig{
		Name:     "noretry-test",
		Subjects: []string{subject("noretry-test")},
		Backoff:  []time.Duration{100 * time.Millisecond},
	}, func(_ context.Context, _ string, data []byte) error {
		ev, err := decode(data)
		if err != nil {
			return err
		}
		if ev.CheckID != 4 {
			return nil
		}
		attempts <- struct{}{}
		return errors.Join(nats.ErrNoRetry, errors.New("bad payload"))
	})
	s.Require().NoError(err)

	want := events.NewCheckAdded(4, 5, 2)
	s.Require().NoError(client.Publish(s.ctx, subject("noretry-test"), want))

	select {
	case <-attempts:
	case <-time.After(5 * time.Second):
		s.Require().Fail("delivery attempt missing")
	}
	s.Eventually(func() bool {
		return testutil.ToFloat64(m.Consumed().WithLabelValues(subject("noretry-test"), nats.ResultDLQ)) == 1
	}, 5*time.Second, 50*time.Millisecond)
	select {
	case <-attempts:
		s.Fail("delivered again")
	case <-time.After(500 * time.Millisecond):
	}
}

// TestStopWaitsForInFlight: Stop returns only after the running handler
// finished and its ack was sent, so a restart does not redeliver the event.
func (s *NatsSuite) TestStopWaitsForInFlight() {
	client := s.newClient(nil)

	started := make(chan struct{})
	release := make(chan struct{})
	var handled int
	consumer, err := client.Consume(s.ctx, nats.ConsumerConfig{
		Name: "drain-test", Subjects: []string{subject("drain-test")},
	}, func(_ context.Context, _ string, data []byte) error {
		ev, err := decode(data)
		if err != nil {
			return err
		}
		if ev.CheckID != 5 {
			return nil
		}
		handled++
		close(started)
		<-release
		return nil
	})
	s.Require().NoError(err)

	want := events.NewCheckAdded(5, 5, 2)
	s.Require().NoError(client.Publish(s.ctx, subject("drain-test"), want))
	<-started

	stopped := make(chan error, 1)
	go func() { stopped <- consumer.Stop(s.ctx) }()
	select {
	case <-stopped:
		s.Fail("Stop returned while the handler was running")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-stopped:
		s.NoError(err)
	case <-time.After(5 * time.Second):
		s.Fail("Stop did not return")
	}

	// A fresh consumer with the same durable name sees nothing: the ack
	// went out before Stop returned.
	got := newReceived()
	_, err = client.Consume(s.ctx, nats.ConsumerConfig{
		Name: "drain-test", Subjects: []string{subject("drain-test")},
	}, func(_ context.Context, _ string, data []byte) error {
		ev, err := decode(data)
		if err != nil {
			return err
		}
		if ev.CheckID == 5 {
			got.add(ev)
		}
		return nil
	})
	s.Require().NoError(err)
	got.none(s, 500*time.Millisecond)
	s.Equal(1, handled)
}

// TestQueueSharing: two instances with the same durable name share the
// events, each event handled once.
func (s *NatsSuite) TestQueueSharing() {
	received := make(chan int, 8)
	for i := range 2 {
		member := s.newClient(nil)
		_, err := member.Consume(s.ctx, nats.ConsumerConfig{
			Name: "share-test", Subjects: []string{subject("share-test")},
		}, func(_ context.Context, _ string, data []byte) error {
			ev, err := decode(data)
			if err != nil {
				return err
			}
			if ev.CheckID == 6 {
				received <- i
			}
			return nil
		})
		s.Require().NoError(err)
	}

	publisher := s.newClient(nil)
	for range 4 {
		s.Require().NoError(publisher.Publish(s.ctx, subject("share-test"), events.NewCheckAdded(6, 1, 1)))
	}
	for range 4 {
		select {
		case <-received:
		case <-time.After(5 * time.Second):
			s.Fail("event not received")
		}
	}
	select {
	case got := <-received:
		s.Failf("event delivered twice", "member %d", got)
	case <-time.After(300 * time.Millisecond):
	}
}

func (s *NatsSuite) TestNewBadURL() {
	_, err := nats.New(slogdiscard.NewDiscardLogger(), config.NatsConfig{URL: "nats://127.0.0.1:1", Name: "test"})
	s.Error(err)
}

// jetStream opens a raw JetStream handle on the same server for
// assertions on the streams.
func (s *NatsSuite) jetStream(client *nats.Client) jetstream.JetStream {
	js, err := client.RawJetStream()
	s.Require().NoError(err)
	return js
}

// CoreFallbackSuite runs the client against a server without JetStream:
// the client warns and works with core NATS (publish, queue subscribe).
type CoreFallbackSuite struct {
	suite.Suite

	ctx context.Context
	cfg config.NatsConfig
}

func TestCoreFallbackSuite(t *testing.T) {
	suite.Run(t, new(CoreFallbackSuite))
}

func (s *CoreFallbackSuite) SetupSuite() {
	s.ctx = context.Background()
	s.cfg = config.NatsConfig{URL: startNATS(&s.Suite, s.ctx, false), Name: "test"}
}

func (s *CoreFallbackSuite) TestFallback() {
	for _, delivery := range []config.NatsDelivery{config.NatsDeliveryJetStream, config.NatsDeliveryCore} {
		s.Run(string(delivery), func() {
			cfg := s.cfg
			cfg.Delivery = delivery

			log := slogdiscard.NewDiscardLogger()
			subscriber, err := nats.New(log, cfg)
			s.Require().NoError(err)
			defer func() { _ = subscriber.Close() }()
			s.False(subscriber.JetStream())

			received := make(chan events.CheckAdded, 1)
			consumer, err := subscriber.Consume(s.ctx, nats.ConsumerConfig{
				Name: "core", Subjects: []string{events.SubjectCheckAdded},
			}, func(_ context.Context, _ string, data []byte) error {
				var ev events.CheckAdded
				if err := json.Unmarshal(data, &ev); err != nil {
					return err
				}
				received <- ev
				return nil
			})
			s.Require().NoError(err)
			// Core NATS has no persistence: make sure the subscription is
			// registered on the server before publishing.
			s.Require().NoError(subscriber.Flush())

			publisher, err := nats.New(log, cfg)
			s.Require().NoError(err)
			defer func() { _ = publisher.Close() }()

			want := events.NewCheckAdded(77, 5, 2)
			s.Require().NoError(publisher.Publish(s.ctx, want.Subject(), want))
			select {
			case got := <-received:
				s.Equal(want, got)
			case <-time.After(5 * time.Second):
				s.Fail("event not received")
			}
			s.NoError(consumer.Stop(s.ctx))
		})
	}
}
