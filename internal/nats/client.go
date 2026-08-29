package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/nats-io/nats.go"
)

const (
	// reconnectWait is the pause between reconnect attempts; the client
	// reconnects forever, so a broker restart never strands a process.
	reconnectWait = 2 * time.Second
	// drainTimeout bounds Close: in-flight subscription handlers get this
	// long to finish before the connection is closed anyway.
	drainTimeout = 30 * time.Second
	// handlerTimeout bounds one subscription handler call (DB work etc.).
	handlerTimeout = 30 * time.Second
)

// Client is a thin wrapper over a core NATS connection: JSON publish,
// subscribe with panic recovery and per-message timeout, drain on close.
// Delivery is core NATS (at-most-once): an event published while no
// subscriber is connected is lost.
type Client struct {
	conn *nats.Conn
	log  *slog.Logger
	// closed is closed once the connection is closed for good (after Close
	// or after the server became unreachable and reconnects were given up).
	closed chan struct{}
}

func New(log *slog.Logger, cfg config.NatsConfig) (*Client, error) {
	const op = "nats.New"

	client := &Client{
		log:    log.With(slog.String("component", "nats")),
		closed: make(chan struct{}),
	}

	opts := []nats.Option{
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

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	client.conn = conn

	return client, nil
}

// Closed is closed when the connection is closed for good; a worker that
// cannot work without the broker should exit when it fires.
func (c *Client) Closed() <-chan struct{} { return c.closed }

// Publish implements events.Publisher: the payload is JSON-encoded and sent
// on subject with core NATS (at-most-once) delivery.
func (c *Client) Publish(_ context.Context, subject string, payload any) error {
	const op = "nats.Publish"

	if err := c.PublishJSON(subject, payload); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// Handler processes one message; ctx carries handlerTimeout.
type Handler func(ctx context.Context, data []byte) error

// Subscribe delivers every message on subject to handler as raw JSON. The
// subscription is released by Close.
func (c *Client) Subscribe(subject string, handler Handler) (*nats.Subscription, error) {
	const op = "nats.Subscribe"

	sub, err := c.conn.Subscribe(subject, c.msgHandler(subject, handler))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return sub, nil
}

// QueueSubscribe is Subscribe within a queue group: a message is delivered
// to one member of the group, so several instances of a worker share the
// load instead of each handling every message.
func (c *Client) QueueSubscribe(subject, queue string, handler Handler) (*nats.Subscription, error) {
	const op = "nats.QueueSubscribe"

	sub, err := c.conn.QueueSubscribe(subject, queue, c.msgHandler(subject, handler))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return sub, nil
}

// msgHandler adapts handler to a NATS callback: a panic in the handler is
// logged instead of killing the process, and every call gets its own
// timeout. Errors are logged; core NATS has no redelivery to request.
func (c *Client) msgHandler(subject string, handler Handler) nats.MsgHandler {
	log := c.log.With(slog.String("subject", subject))
	return func(msg *nats.Msg) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("handler panic",
					slog.Any("panic", r),
					slog.String("stack", string(debug.Stack())),
				)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
		defer cancel()

		if err := handler(ctx, msg.Data); err != nil {
			log.Error("handler error", slogger.Err(err))
		}
	}
}

// Flush waits until the server acknowledged everything sent so far
// (including subscriptions).
func (c *Client) Flush() error {
	return c.conn.Flush()
}

// Close drains the connection (pending publishes are flushed, subscriptions
// are unsubscribed and their in-flight handlers finish) and closes it. It
// gives up after drainTimeout and implements io.Closer for app.Closers.
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

func (c *Client) PublishJSON(subject string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.conn.Publish(subject, jsonData)
}

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
