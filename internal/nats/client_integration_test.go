//go:build integration

package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/nats"
	"github.com/PritOriginal/problem-map-server/pkg/logger/slogdiscard"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const natsImage = "nats:2-alpine"

// NatsSuite runs the client against a real NATS server: an event published
// by one client reaches a subscriber on another.
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

	container, err := testcontainers.GenericContainer(s.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        natsImage,
			ExposedPorts: []string{"4222/tcp"},
			WaitingFor:   wait.ForLog("Server is ready").WithStartupTimeout(time.Minute),
		},
		Started: true,
	})
	s.Require().NoError(err, "start nats container")
	testcontainers.CleanupContainer(s.T(), container)

	endpoint, err := container.PortEndpoint(s.ctx, "4222/tcp", "nats")
	s.Require().NoError(err)
	s.cfg = config.NatsConfig{URL: endpoint, Name: "test"}
}

func (s *NatsSuite) TestPublishSubscribe() {
	log := slogdiscard.NewDiscardLogger()

	subscriber, err := nats.New(log, s.cfg)
	s.Require().NoError(err)
	defer func() { _ = subscriber.Close() }()

	publisher, err := nats.New(log, s.cfg)
	s.Require().NoError(err)
	defer func() { _ = publisher.Close() }()

	received := make(chan events.CheckAdded, 1)
	_, err = subscriber.Subscribe(events.SubjectCheckAdded, func(_ context.Context, data []byte) error {
		var ev events.CheckAdded
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		received <- ev
		return nil
	})
	s.Require().NoError(err)
	// Core NATS has no persistence: make sure the subscription is registered
	// on the server before publishing.
	s.Require().NoError(subscriber.Flush())

	want := events.NewCheckAdded(77, 5, 2)
	var pub events.Publisher = publisher
	s.Require().NoError(pub.Publish(s.ctx, want.Subject(), want))

	select {
	case got := <-received:
		s.Equal(want, got)
	case <-time.After(5 * time.Second):
		s.Fail("event not received")
	}

	// Another subject is not delivered to this subscriber.
	s.Require().NoError(pub.Publish(s.ctx, events.SubjectMarkStatusChanged,
		events.NewMarkStatusChanged(1, models.UnconfirmedStatus, models.ConfirmedStatus, 1)))
	select {
	case got := <-received:
		s.Failf("unexpected event", "%+v", got)
	case <-time.After(300 * time.Millisecond):
	}
}

func (s *NatsSuite) TestNewBadURL() {
	_, err := nats.New(slogdiscard.NewDiscardLogger(), config.NatsConfig{URL: "nats://127.0.0.1:1", Name: "test"})
	s.Error(err)
}
