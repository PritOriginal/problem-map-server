package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/google/uuid"
)

// WebhookQueueGroup is the NATS queue group of the webhook consumer; it is
// separate from QueueGroup so that every event reaches both the
// notification and the webhook consumers exactly once each.
const WebhookQueueGroup = "webhooks"

// WebhookSubjects are the wildcard subscriptions of the webhook consumer:
// every current and future event of the three domains is forwarded.
var WebhookSubjects = []string{"mark.>", "task.>", "check.>"}

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

// Subscribe registers Handle for every wildcard subject within
// WebhookQueueGroup. The subject of the delivered message (not the
// subscription pattern) is what the dispatcher receives, so QueueSubscribe
// implementations must pass it (see natsSubscriber).
func (r *WebhookRouter) Subscribe(sub SubjectSubscriber) error {
	for _, pattern := range WebhookSubjects {
		if _, err := sub.QueueSubscribeSubject(pattern, WebhookQueueGroup, r.Handle); err != nil {
			return fmt.Errorf("subscribe %s: %w", pattern, err)
		}
	}
	return nil
}

// Handle validates the event header and dispatches the payload. A payload
// without event_id gets a fresh one (and cannot be deduplicated on
// redelivery); a newer schema version is rejected like in Router.
func (r *WebhookRouter) Handle(ctx context.Context, subject string, data []byte) error {
	var header events.Header
	if err := json.Unmarshal(data, &header); err != nil {
		return fmt.Errorf("decode %s: %w", subject, err)
	}
	if err := header.CheckVersion(); err != nil {
		return fmt.Errorf("decode %s: %w", subject, err)
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
