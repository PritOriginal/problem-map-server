// Package nats is the broker client of the domain events: JSON publish
// (JetStream with server-side deduplication, or core NATS as a fallback),
// a durable pull consumer with explicit acks, redelivery with backoff and a
// dead-letter stream, plus drain on close.
package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// reconnectWait is the pause between reconnect attempts; the client
	// reconnects forever, so a broker restart never strands a process.
	reconnectWait = 2 * time.Second
	// drainTimeout bounds Close: in-flight subscription handlers get this
	// long to finish before the connection is closed anyway.
	drainTimeout = 30 * time.Second
	// handlerTimeout bounds one message handler call (DB work etc.).
	handlerTimeout = 30 * time.Second
	// setupTimeout bounds the JetStream API calls made at startup (account
	// probe, stream and consumer creation).
	setupTimeout = 10 * time.Second
	// publishTimeout bounds a JetStream publish (waiting for the PubAck)
	// when the caller's context has no deadline of its own.
	publishTimeout = 5 * time.Second
)

// Client is the broker connection. With JetStream (the default) Publish
// stores the event in StreamEvents before returning, so a consumer that
// is down at that moment still receives it (at-least-once); the event_id
// of the payload is sent as Nats-Msg-Id, so a retried publish inside the
// deduplication window is dropped by the server. When JetStream is
// disabled (config) or the server has it turned off (a warning at start)
// the client publishes with core NATS: at-most-once, no persistence.
type Client struct {
	conn *nats.Conn
	// js is nil in core mode.
	js      jetstream.JetStream
	log     *slog.Logger
	metrics *Metrics
	// closed is closed once the connection is closed for good (after Close
	// or after the server became unreachable and reconnects were given up).
	closed chan struct{}
}

// Option customises New.
type Option func(*Client)

// WithMetrics makes the client record its counters on m.
func WithMetrics(m *Metrics) Option {
	return func(c *Client) { c.metrics = m }
}

func New(log *slog.Logger, cfg config.NatsConfig, opts ...Option) (*Client, error) {
	const op = "nats.New"

	client := &Client{
		log:    log.With(slog.String("component", "nats")),
		closed: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(client)
	}

	connOpts := []nats.Option{
		nats.Name(cfg.Name),
		nats.NoEcho(),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(reconnectWait),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				client.log.Warn("disconnected, reconnecting", slogger.Err(err))
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			client.log.Info("reconnected", slog.String("url", nc.ConnectedUrl()))
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			if err := nc.LastError(); err != nil {
				client.log.Error("connection closed", slogger.Err(err))
			}
			close(client.closed)
		}),
	}

	conn, err := nats.Connect(cfg.URL, connOpts...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	client.conn = conn

	if cfg.JetStream() {
		if err := client.initJetStream(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	} else {
		client.log.Warn("nats.delivery is core: events are delivered at most once")
	}

	return client, nil
}

// initJetStream probes the server for JetStream and ensures the streams.
// A server without JetStream is not an error: the client logs a warning
// and stays in core mode.
func (c *Client) initJetStream() error {
	js, err := jetstream.New(c.conn)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	if _, err := js.AccountInfo(ctx); err != nil {
		if errors.Is(err, jetstream.ErrJetStreamNotEnabled) || errors.Is(err, jetstream.ErrJetStreamNotEnabledForAccount) {
			c.log.Warn("jetstream is not enabled on the server, falling back to core nats (at-most-once delivery)",
				slogger.Err(err))
			return nil
		}
		return fmt.Errorf("jetstream account info: %w", err)
	}

	if err := ensureStreams(ctx, js); err != nil {
		return err
	}
	c.js = js
	c.log.Info("jetstream streams ready", slog.String("events", StreamEvents), slog.String("dlq", StreamDLQ))
	return nil
}

// Closed is closed when the connection is closed for good; a worker that
// cannot work without the broker should exit when it fires.
func (c *Client) Closed() <-chan struct{} { return c.closed }

// JetStream reports whether events go through JetStream (false in core
// mode).
func (c *Client) JetStream() bool { return c.js != nil }

// ErrNoJetStream is returned by RawJetStream in core mode.
var ErrNoJetStream = errors.New("jetstream is not available")

// RawJetStream exposes the JetStream handle for tooling (stream
// inspection, DLQ replay); ErrNoJetStream in core mode.
func (c *Client) RawJetStream() (jetstream.JetStream, error) {
	if c.js == nil {
		return nil, ErrNoJetStream
	}
	return c.js, nil
}

// identified is implemented by payloads that carry a stable id (every
// domain event via events.Header); it becomes the Nats-Msg-Id.
type identified interface {
	ID() string
}

// Publish implements events.Publisher: the payload is JSON-encoded and
// stored in StreamEvents (or sent with core NATS in core mode). A payload
// implementing ID() is deduplicated by the server within the duplicates
// window; a duplicate is not an error.
func (c *Client) Publish(ctx context.Context, subject string, payload any) error {
	const op = "nats.Publish"

	data, err := json.Marshal(payload)
	if err != nil {
		c.metrics.recordPublished(subject, ResultError)
		return fmt.Errorf("%s: %w", op, err)
	}

	if c.js == nil {
		if err := c.conn.Publish(subject, data); err != nil {
			c.metrics.recordPublished(subject, ResultError)
			return fmt.Errorf("%s: %w", op, err)
		}
		c.metrics.recordPublished(subject, ResultOK)
		return nil
	}

	var opts []jetstream.PublishOpt
	if id, ok := payload.(identified); ok && id.ID() != "" {
		opts = append(opts, jetstream.WithMsgID(id.ID()))
	}

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, publishTimeout)
		defer cancel()
	}

	ack, err := c.js.Publish(ctx, subject, data, opts...)
	if err != nil {
		c.metrics.recordPublished(subject, ResultError)
		return fmt.Errorf("%s: %w", op, err)
	}
	if ack.Duplicate {
		c.metrics.recordPublished(subject, ResultDuplicate)
		c.log.Debug("duplicate event dropped by the server", slog.String("subject", subject))
		return nil
	}
	c.metrics.recordPublished(subject, ResultOK)
	return nil
}

// Flush waits until the server acknowledged everything sent so far
// (including subscriptions).
func (c *Client) Flush() error {
	return c.conn.Flush()
}

// Close drains the connection (pending publishes are flushed, subscriptions
// are unsubscribed and their in-flight handlers finish) and closes it. It
// gives up after drainTimeout and implements io.Closer for app.Closers.
// A Consumer must be stopped before its client is closed so that the acks
// of in-flight messages reach the server.
func (c *Client) Close() error {
	const op = "nats.Close"

	if c.conn.IsClosed() {
		return nil
	}
	if err := c.conn.Drain(); err != nil {
		c.conn.Close()
		return fmt.Errorf("%s: %w", op, err)
	}

	select {
	case <-c.closed:
		return nil
	case <-time.After(drainTimeout):
		c.conn.Close()
		return fmt.Errorf("%s: %w", op, errors.New("drain timed out"))
	}
}

// RequestJSON sends a JSON request on subject and returns the raw reply.
func (c *Client) RequestJSON(subject string, request any, timeout time.Duration) ([]byte, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	msg, err := c.conn.Request(subject, jsonData, timeout)
	if err != nil {
		return nil, err
	}

	return msg.Data, nil
}
