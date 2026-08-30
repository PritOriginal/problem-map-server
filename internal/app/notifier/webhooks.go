package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/nats"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/google/uuid"
)

// WebhookConsumerName is the durable JetStream consumer (and the core NATS
// queue group) of the webhook dispatcher; it is separate from ConsumerName
// so that every event reaches both the notification and the webhook
// consumers exactly once each.
const WebhookConsumerName = "webhooks"

// WebhookSubjects are the wildcard subscriptions of the webhook consumer:
// every current and future event of the four domains is forwarded.
var WebhookSubjects = []string{"mark.>", "task.>", "check.>", "badge.>"}

// Dispatcher fans an event out to the subscribed webhooks (usecase.Webhooks).
type Dispatcher interface {
	Dispatch(ctx context.Context, subject, eventID string, data json.RawMessage) error
	RetryDue(ctx context.Context, limit int) (int, error)
	// PruneDeliveries removes old delivery rows (retention).
	PruneDeliveries(ctx context.Context) (int64, error)
}

// PruneInterval is how often the delivery log is pruned of old rows.
const PruneInterval = time.Hour

// WebhookRouter forwards raw events to the Dispatcher without decoding
// them into typed events: the payload is passed through as is.
type WebhookRouter struct {
	log        *slog.Logger
	dispatcher Dispatcher
}

func NewWebhookRouter(log *slog.Logger, dispatcher Dispatcher) *WebhookRouter {
	return &WebhookRouter{log: log, dispatcher: dispatcher}
}

// Handle validates the event header and dispatches the payload
// (nats.MsgHandler). A payload without event_id gets a fresh one (and
// cannot be deduplicated on redelivery); an undecodable payload or a newer
// schema version is a nats.ErrNoRetry error like in Router (the event is
// dead-lettered), a dispatch error is returned as is so that the event is
// redelivered.
func (r *WebhookRouter) Handle(ctx context.Context, subject string, data []byte) error {
	var header events.Header
	if err := json.Unmarshal(data, &header); err != nil {
		return fmt.Errorf("%w: decode %s: %w", nats.ErrNoRetry, subject, err)
	}
	if err := header.CheckVersion(); err != nil {
		return fmt.Errorf("%w: decode %s: %w", nats.ErrNoRetry, subject, err)
	}
	if header.EventID == "" {
		header.EventID = uuid.NewString()
		r.log.Warn("event without event_id, generated one", slog.String("subject", subject), slog.String("event_id", header.EventID))
	}

	if err := r.dispatcher.Dispatch(ctx, subject, header.EventID, json.RawMessage(data)); err != nil {
		return fmt.Errorf("dispatch %s: %w", subject, err)
	}
	return nil
}

// RetryLoop attempts due deliveries every interval and prunes the delivery
// log every PruneInterval until ctx is done.
func (r *WebhookRouter) RetryLoop(ctx context.Context, interval time.Duration, batch int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	pruner := time.NewTicker(PruneInterval)
	defer pruner.Stop()

	r.prune(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.retry(ctx, batch)
		case <-pruner.C:
			r.prune(ctx)
		}
	}
}

func (r *WebhookRouter) prune(ctx context.Context) {
	n, err := r.dispatcher.PruneDeliveries(ctx)
	if err != nil {
		r.log.Warn("failed to prune webhook deliveries", slogger.Err(err))
		return
	}
	if n > 0 {
		r.log.Info("old webhook deliveries pruned", slog.Int64("deleted", n))
	}
}

func (r *WebhookRouter) retry(ctx context.Context, batch int) {
	n, err := r.dispatcher.RetryDue(ctx, batch)
	if err != nil {
		r.log.Warn("webhook retries finished with errors", slog.Int("attempted", n), slogger.Err(err))
		return
	}
	if n > 0 {
		r.log.Info("webhook retries attempted", slog.Int("attempted", n))
	}
}
