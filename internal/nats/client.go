package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/nats-io/nats.go"
)

type Client struct {
	conn *nats.Conn
	log  *slog.Logger
}

func New(log *slog.Logger, cfg config.NatsConfig) (*Client, error) {
	const op = "nats.New"

	opts := []nats.Option{
		nats.Name(cfg.Name),
		// nats.MaxReconnects(cfg.MaxReconnects),
		// nats.ReconnectWait(cfg.ReconnectWait),
		// nats.Timeout(cfg.Timeout),
		nats.NoEcho(),
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	client := &Client{
		conn: conn,
		log:  log.With(slog.String("component", "nats")),
	}

	return client, nil
}

// Publish implements events.Publisher: the payload is JSON-encoded and sent
// on subject with core NATS (at-most-once) delivery.
func (c *Client) Publish(_ context.Context, subject string, payload any) error {
	const op = "nats.Publish"

	if err := c.PublishJSON(subject, payload); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// Subscribe delivers every message on subject to handler as raw JSON. The
// subscription is released by Close.
func (c *Client) Subscribe(subject string, handler func(ctx context.Context, data []byte) error) (*nats.Subscription, error) {
	const op = "nats.Subscribe"

	sub, err := c.SubscribeJSON(subject, func(_ *nats.Msg, data []byte) error {
		return handler(context.Background(), data)
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return sub, nil
}

// Flush waits until the server acknowledged everything sent so far
// (including subscriptions).
func (c *Client) Flush() error {
	return c.conn.Flush()
}

// Close drains the connection and implements io.Closer for app.Closers.
func (c *Client) Close() error {
	c.conn.Close()
	return nil
}

func (c *Client) PublishJSON(subject string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.conn.Publish(subject, jsonData)
}

func (c *Client) SubscribeJSON(subject string, handler func(msg *nats.Msg, data []byte) error) (*nats.Subscription, error) {
	return c.conn.Subscribe(subject, func(msg *nats.Msg) {
		if err := handler(msg, msg.Data); err != nil {
			c.log.Error("handler error", slog.String("subject", subject), slogger.Err(err))
		}
	})
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
