package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// ErrNoRetry marks a handler failure that a redelivery cannot fix (an
// undecodable payload, an unknown subject): the message goes straight to
// the dead-letter stream instead of being retried.
var ErrNoRetry = errors.New("no retry")

// MsgHandler processes one event; ctx carries handlerTimeout. A nil error
// acknowledges the message, an error requests a redelivery (or, wrapped
// in ErrNoRetry, dead-letters it).
type MsgHandler func(ctx context.Context, subject string, data []byte) error

// Defaults of ConsumerConfig.
const (
	DefaultMaxDeliver = 5
	// defaultAckWait leaves the handler its full timeout, the dead-letter
	// publish its own (publishTimeout) and a margin for the ack to travel
	// before the server redelivers.
	defaultAckWait = handlerTimeout + publishTimeout + 5*time.Second
)

// DefaultBackoff is the delay before the n-th redelivery (the last value
// repeats): a transient failure (DB hiccup) is retried quickly, a longer
// outage is not hammered.
var DefaultBackoff = []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute}

// ConsumerConfig describes a durable consumer of StreamEvents.
type ConsumerConfig struct {
	// Name is the durable name (and the queue group in core mode); several
	// processes with the same Name share the events.
	Name string
	// Subjects filters the events delivered (e.g. "mark.status_changed").
	Subjects []string
	// MaxDeliver is the number of attempts before an event is
	// dead-lettered; DefaultMaxDeliver when 0.
	MaxDeliver int
	// Backoff is the delay before the n-th redelivery; DefaultBackoff when
	// empty.
	Backoff []time.Duration
	// AckWait is how long the server waits for an ack before it redelivers
	// (a crashed worker); defaultAckWait when 0.
	AckWait time.Duration
}

func (cfg ConsumerConfig) withDefaults() ConsumerConfig {
	if cfg.MaxDeliver <= 0 {
		cfg.MaxDeliver = DefaultMaxDeliver
	}
	if len(cfg.Backoff) == 0 {
		cfg.Backoff = DefaultBackoff
	}
	if cfg.AckWait <= 0 {
		cfg.AckWait = defaultAckWait
	}
	return cfg
}

// backoff returns the delay before the next delivery after the delivered-th
// attempt failed.
func (cfg ConsumerConfig) backoff(delivered uint64) time.Duration {
	n := uint64(len(cfg.Backoff)) //nolint:gosec // len is never negative
	if delivered == 0 {
		delivered = 1
	}
	if delivered > n {
		delivered = n
	}
	return cfg.Backoff[delivered-1]
}

// exhausted reports whether the delivered-th attempt was the last one.
func (cfg ConsumerConfig) exhausted(delivered uint64) bool {
	if cfg.MaxDeliver <= 0 {
		return false
	}
	return delivered >= uint64(cfg.MaxDeliver) //nolint:gosec // checked positive above
}

// Consumer receives the events of StreamEvents through a durable pull
// consumer: every message is acked after the handler succeeded, nak'ed
// with a backoff after it failed and moved to StreamDLQ after
// ConsumerConfig.MaxDeliver failed attempts. Handlers must be idempotent:
// a message whose ack was lost (crash, network) is delivered again.
//
// In core mode (no JetStream) the consumer is a queue subscription: no
// acks, no redelivery, a failure is only logged.
type Consumer struct {
	log     *slog.Logger
	metrics *Metrics
	cfg     ConsumerConfig
	handler MsgHandler

	// consumeCtx is the JetStream subscription; nil in core mode.
	consumeCtx jetstream.ConsumeContext
	dlq        DLQPublisher
	// subs are the core subscriptions; empty in JetStream mode.
	subs []*nats.Subscription
	// inflight counts handler calls so Stop can wait for them.
	inflight sync.WaitGroup
}

// DLQPublisher is the part of jetstream.JetStream used to dead-letter a
// message.
type DLQPublisher interface {
	PublishMsg(ctx context.Context, msg *nats.Msg, opts ...jetstream.PublishOpt) (*jetstream.PubAck, error)
}

// Consume starts delivering the events matching cfg.Subjects to handler
// and returns immediately; ctx bounds the setup only. Stop the consumer
// before closing the client.
func (c *Client) Consume(ctx context.Context, cfg ConsumerConfig, handler MsgHandler) (*Consumer, error) {
	const op = "nats.Consume"

	cfg = cfg.withDefaults()
	if cfg.Name == "" {
		return nil, fmt.Errorf("%s: consumer name is empty", op)
	}
	if len(cfg.Subjects) == 0 {
		return nil, fmt.Errorf("%s: no subjects", op)
	}

	consumer := &Consumer{
		log:     c.log.With(slog.String("consumer", cfg.Name)),
		metrics: c.metrics,
		cfg:     cfg,
		handler: handler,
		dlq:     c.js,
	}

	if c.js == nil {
		if err := consumer.subscribeCore(c.conn); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		return consumer, nil
	}

	if err := consumer.subscribeJetStream(ctx, c.js); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return consumer, nil
}

func (c *Consumer) subscribeJetStream(ctx context.Context, js jetstream.JetStream) error {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, setupTimeout)
		defer cancel()
	}

	cons, err := js.CreateOrUpdateConsumer(ctx, StreamEvents, jetstream.ConsumerConfig{
		Durable:        c.cfg.Name,
		Description:    "problem-map " + c.cfg.Name,
		FilterSubjects: c.cfg.Subjects,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
		AckWait:        c.cfg.AckWait,
		MaxDeliver:     c.cfg.MaxDeliver,
	})
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}

	consumeCtx, err := cons.Consume(c.handle, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		c.log.Warn("consume error", slogger.Err(err))
	}))
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	c.consumeCtx = consumeCtx
	return nil
}

func (c *Consumer) subscribeCore(conn *nats.Conn) error {
	c.log.Warn("core nats mode: events are not acknowledged and never redelivered")
	for _, subject := range c.cfg.Subjects {
		sub, err := conn.QueueSubscribe(subject, c.cfg.Name, c.handleCore)
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
		c.subs = append(c.subs, sub)
	}
	return nil
}

// handleCore is the core NATS callback: errors are logged, there is no
// redelivery to request.
func (c *Consumer) handleCore(msg *nats.Msg) {
	c.inflight.Add(1)
	defer c.inflight.Done()

	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	if err := c.call(ctx, msg.Subject, msg.Data); err != nil {
		c.metrics.recordConsumed(msg.Subject, ResultError)
		c.log.Error("handler error", slog.String("subject", msg.Subject), slogger.Err(err))
		return
	}
	c.metrics.recordConsumed(msg.Subject, ResultOK)
}

// handle is the JetStream callback: it runs the handler and decides the
// fate of the message (ack, nak with backoff, dead-letter).
func (c *Consumer) handle(msg jetstream.Msg) {
	c.inflight.Add(1)
	defer c.inflight.Done()

	subject := msg.Subject()
	log := c.log.With(slog.String("subject", subject))

	var delivered uint64 = 1
	if meta, err := msg.Metadata(); err == nil {
		delivered = meta.NumDelivered
		log = log.With(slog.Uint64("stream_seq", meta.Sequence.Stream), slog.Uint64("delivered", delivered))
	} else {
		log.Warn("message without metadata", slogger.Err(err))
	}
	if delivered > 1 {
		c.metrics.recordRedelivery()
	}

	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	err := c.call(ctx, subject, msg.Data())
	switch {
	case err == nil:
		if err := msg.Ack(); err != nil {
			// The handler did its work; the message is redelivered and
			// the (idempotent) handler sees it again.
			log.Error("ack failed, message will be redelivered", slogger.Err(err))
		}
		c.metrics.recordConsumed(subject, ResultAck)

	case errors.Is(err, ErrNoRetry), c.cfg.exhausted(delivered):
		c.deadLetter(log, msg, delivered, err)

	default:
		delay := c.cfg.backoff(delivered)
		log.Warn("handler error, message will be redelivered", slog.Duration("after", delay), slogger.Err(err))
		if err := msg.NakWithDelay(delay); err != nil {
			log.Error("nak failed", slogger.Err(err))
		}
		c.metrics.recordConsumed(subject, ResultNak)
	}
}

// deadLetter copies msg into StreamDLQ and terminates it. When the copy
// fails the message is left to the server (nak): it comes back if
// deliveries remain and is otherwise logged in full so an operator can
// replay it by hand. The publish gets its own deadline: the handler's
// context is usually already expired when the handler timed out.
func (c *Consumer) deadLetter(log *slog.Logger, msg jetstream.Msg, delivered uint64, cause error) {
	subject := msg.Subject()

	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()

	dlqMsg := &nats.Msg{
		Subject: DLQSubjectPrefix + subject,
		Data:    msg.Data(),
		Header:  nats.Header{},
	}
	for k, v := range msg.Headers() {
		dlqMsg.Header[k] = v
	}
	dlqMsg.Header.Set(HeaderDLQSubject, subject)
	dlqMsg.Header.Set(HeaderDLQConsumer, c.cfg.Name)
	dlqMsg.Header.Set(HeaderDLQDeliveries, strconv.FormatUint(delivered, 10))
	dlqMsg.Header.Set(HeaderDLQError, cause.Error())
	if meta, err := msg.Metadata(); err == nil {
		dlqMsg.Header.Set(HeaderDLQStream, meta.Stream)
		dlqMsg.Header.Set(HeaderDLQStreamSeq, strconv.FormatUint(meta.Sequence.Stream, 10))
	}
	var opts []jetstream.PublishOpt
	if id := msg.Headers().Get(jetstream.MsgIDHeader); id != "" {
		dlqMsg.Header.Set(HeaderDLQMsgID, id)
		opts = append(opts, jetstream.WithMsgID("dlq-"+c.cfg.Name+"-"+id))
	}

	if _, err := c.dlq.PublishMsg(ctx, dlqMsg, opts...); err != nil {
		log.Error("failed to dead-letter message, event may be lost",
			slog.String("payload", string(msg.Data())),
			slog.String("cause", cause.Error()),
			slogger.Err(err),
		)
		if err := msg.Nak(); err != nil {
			log.Error("nak failed", slogger.Err(err))
		}
		c.metrics.recordConsumed(subject, ResultError)
		return
	}

	log.Error("message dead-lettered", slog.String("dlq_subject", dlqMsg.Subject), slogger.Err(cause))
	if err := msg.Term(); err != nil {
		log.Error("term failed", slogger.Err(err))
	}
	c.metrics.recordConsumed(subject, ResultDLQ)
}

// call runs the handler, turning a panic into an error so that one bad
// event neither kills the worker nor is redelivered forever.
func (c *Consumer) call(ctx context.Context, subject string, data []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("handler panic",
				slog.String("subject", subject),
				slog.Any("panic", r),
				slog.String("stack", string(debug.Stack())),
			)
			err = fmt.Errorf("%w: handler panic: %v", ErrNoRetry, r)
		}
	}()
	return c.handler(ctx, subject, data)
}

// Stop stops receiving new messages, waits (bounded by ctx) for the
// in-flight handlers to finish and their acks to be sent, and releases
// the subscription. It is safe to call more than once.
func (c *Consumer) Stop(ctx context.Context) error {
	const op = "nats.Consumer.Stop"

	if c.consumeCtx != nil {
		c.consumeCtx.Drain()
		select {
		case <-c.consumeCtx.Closed():
		case <-ctx.Done():
			c.consumeCtx.Stop()
			return fmt.Errorf("%s: %w", op, ctx.Err())
		}
	}

	var errs []error
	for _, sub := range c.subs {
		if err := sub.Unsubscribe(); err != nil && !errors.Is(err, nats.ErrBadSubscription) && !errors.Is(err, nats.ErrConnectionClosed) {
			errs = append(errs, err)
		}
	}
	c.subs = nil

	done := make(chan struct{})
	go func() {
		c.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		errs = append(errs, ctx.Err())
	}

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}
