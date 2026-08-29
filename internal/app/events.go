package app

import (
	"io"
	"log/slog"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/nats"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
)

// NewPublisher builds the domain-event publisher selected by cfg. With an
// empty cfg.URL no broker is used: events are dropped by
// events.NoopPublisher and a warning is logged once at startup. The closer
// is a nil interface for the noop publisher, so it can be passed straight
// to Closers.Add.
func NewPublisher(log *slog.Logger, cfg config.NatsConfig) (events.Publisher, io.Closer) {
	if cfg.URL == "" {
		log.Warn("nats.url is empty: domain events are not published (notifications disabled)")
		return events.NoopPublisher{}, nil
	}

	client, err := nats.New(log, cfg)
	if err != nil {
		log.Error("failed connection to nats", slogger.Err(err))
		panic(err)
	}
	log.Info("NATS connected!")

	return client, client
}
