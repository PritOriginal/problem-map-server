package nats

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/config"
	"github.com/nats-io/nats.go"
)

type Client struct {
	conn *nats.Conn
}

func New(cfg config.NatsConfig) (*Client, error) {
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
	}

	return client, nil
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
			log.Printf("[NATS] Handler error for %s: %v", subject, err)
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

func (c *Client) Close() {
	c.conn.Close()
}
