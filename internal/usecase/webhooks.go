package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/events"
	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/webhooks"
	slogger "github.com/PritOriginal/problem-map-server/pkg/logger"
	"github.com/google/uuid"
	"github.com/guregu/null/v6"
)

type WebhooksRepository interface {
	AddWebhook(ctx context.Context, w models.Webhook) (int64, error)
	GetWebhookById(ctx context.Context, id int) (models.Webhook, error)
	GetWebhooksByOwner(ctx context.Context, ownerUserID int) ([]models.Webhook, error)
	// GetActiveWebhooksForEvent returns the active webhooks subscribed to
	// subject (exactly, by "<prefix>.*" or by "*").
	GetActiveWebhooksForEvent(ctx context.Context, subject string) ([]models.Webhook, error)
	UpdateWebhook(ctx context.Context, id int, upd models.WebhookUpdate) error
	DeleteWebhook(ctx context.Context, id int) error

	// AddDelivery inserts a pending delivery; created is false (and the
	// stored row returned) when the event was already delivered/queued to
	// the webhook.
	AddDelivery(ctx context.Context, d models.WebhookDelivery) (models.WebhookDelivery, bool, error)
	GetDeliveriesByWebhookId(ctx context.Context, webhookID int, p models.Pagination) (models.Page[models.WebhookDelivery], error)
	// ClaimDueDeliveries returns deliveries due for another attempt and
	// postpones them by lease so that no other worker picks them up.
	ClaimDueDeliveries(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.PendingWebhookDelivery, error)
	RecordAttempt(ctx context.Context, deliveryID int64, res models.WebhookAttemptResult) error
	// DeleteDeliveriesBefore removes deliveries created before the time and
	// returns their number.
	DeleteDeliveriesBefore(ctx context.Context, before time.Time) (int64, error)
}

// WebhookSender performs one HTTP delivery attempt (see webhooks.Sender).
type WebhookSender interface {
	Send(ctx context.Context, req webhooks.Request) webhooks.Result
}

// WebhookURLValidator rejects targets a webhook must not point at (see
// webhooks.URLPolicy).
type WebhookURLValidator interface {
	Validate(ctx context.Context, raw string) error
}

type WebhooksRepositories struct {
	Webhooks WebhooksRepository
}

// WebhooksDeps are the collaborators of the webhooks use case. Sender and
// URLs are required; Notifications is optional and receives the
// "webhook disabled" notice for the owner.
type WebhooksDeps struct {
	Sender        WebhookSender
	URLs          WebhookURLValidator
	Notifications NotificationCreator
}

// Webhooks manages webhook subscriptions and delivers events to them.
type Webhooks struct {
	log   *slog.Logger
	repos WebhooksRepositories
	deps  WebhooksDeps
	now   func() time.Time
}

func NewWebhooks(log *slog.Logger, deps WebhooksDeps, repos WebhooksRepositories) *Webhooks {
	return &Webhooks{
		log:   log,
		repos: repos,
		deps:  deps,
		now:   time.Now,
	}
}

// KnownWebhookEvents lists the subjects a webhook may subscribe to.
func KnownWebhookEvents() []string {
	return []string{
		events.SubjectMarkStatusChanged, events.SubjectTaskAssigned, events.SubjectCheckAdded,
		events.SubjectMarkHidden, events.SubjectMarkMerged,
	}
}

// webhookSecretBytes is the entropy of a generated secret (hex-encoded to
// twice as many characters).
const webhookSecretBytes = 32

// maxWebhookErrorLen bounds the error text stored per delivery.
const maxWebhookErrorLen = 1024

// webhookClaimLease is how long a delivery stays claimed by the worker
// attempting it; a crash mid-attempt makes it due again after the lease.
const webhookClaimLease = 2 * time.Minute

// webhookDispatchConcurrency bounds the parallel first attempts of one event.
const webhookDispatchConcurrency = 8

// WebhookDeliveryRetention is how long delivery rows are kept before
// PruneDeliveries removes them.
const WebhookDeliveryRetention = 30 * 24 * time.Hour

// MaxWebhookPayloadBytes caps the event data one delivery may carry: the
// payload is stored per webhook (JSONB) and re-sent on every retry.
const MaxWebhookPayloadBytes = 256 << 10

// ErrWebhookPayloadTooLarge is returned by Dispatch for events larger than
// MaxWebhookPayloadBytes; such an event is dropped, not retried.
var ErrWebhookPayloadTooLarge = fmt.Errorf("%w: webhook payload too large", ErrInvalidArgument)

// Create validates and stores w for the actor. An empty Secret is
// generated; the returned webhook carries it (the only time it is shown).
func (uc *Webhooks) Create(ctx context.Context, actor models.Actor, w models.Webhook) (models.Webhook, error) {
	const op = "usecase.Webhooks.Create"

	if err := uc.deps.URLs.Validate(ctx, w.URL); err != nil {
		return models.Webhook{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if err := models.ValidateWebhookEvents(w.Events, KnownWebhookEvents()); err != nil {
		return models.Webhook{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if w.Secret == "" {
		secret, err := generateSecret()
		if err != nil {
			return models.Webhook{}, fmt.Errorf("%s: %w", op, err)
		}
		w.Secret = secret
	}
	w.OwnerUserID = actor.UserID
	w.Active = true

	id, err := uc.repos.Webhooks.AddWebhook(ctx, w)
	if err != nil {
		return models.Webhook{}, mapRepoErr(op, err)
	}

	stored, err := uc.repos.Webhooks.GetWebhookById(ctx, int(id))
	if err != nil {
		return models.Webhook{}, mapRepoErr(op, err)
	}
	stored.Secret = w.Secret

	return stored, nil
}

func generateSecret() (string, error) {
	buf := make([]byte, webhookSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// List returns the actor's webhooks.
func (uc *Webhooks) List(ctx context.Context, actor models.Actor) ([]models.Webhook, error) {
	const op = "usecase.Webhooks.List"

	hooks, err := uc.repos.Webhooks.GetWebhooksByOwner(ctx, actor.UserID)
	if err != nil {
		return nil, mapRepoErr(op, err)
	}

	return hooks, nil
}

// get loads the webhook and checks that the actor may manage it: the
// owner or an admin.
func (uc *Webhooks) get(ctx context.Context, op string, actor models.Actor, id int) (models.Webhook, error) {
	w, err := uc.repos.Webhooks.GetWebhookById(ctx, id)
	if err != nil {
		return models.Webhook{}, mapRepoErr(op, err)
	}
	if w.OwnerUserID != actor.UserID && actor.Role != models.RoleAdmin {
		return models.Webhook{}, fmt.Errorf("%s: %w", op, ErrForbidden)
	}
	return w, nil
}

// Update changes active and/or events of the webhook.
func (uc *Webhooks) Update(ctx context.Context, actor models.Actor, id int, upd models.WebhookUpdate) (models.Webhook, error) {
	const op = "usecase.Webhooks.Update"

	if upd.IsEmpty() {
		return models.Webhook{}, fmt.Errorf("%s: %w: nothing to update", op, ErrInvalidArgument)
	}
	if upd.Events != nil {
		if err := models.ValidateWebhookEvents(upd.Events, KnownWebhookEvents()); err != nil {
			return models.Webhook{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
		}
	}

	if _, err := uc.get(ctx, op, actor, id); err != nil {
		return models.Webhook{}, err
	}
	if err := uc.repos.Webhooks.UpdateWebhook(ctx, id, upd); err != nil {
		return models.Webhook{}, mapRepoErr(op, err)
	}

	w, err := uc.repos.Webhooks.GetWebhookById(ctx, id)
	if err != nil {
		return models.Webhook{}, mapRepoErr(op, err)
	}

	return w, nil
}

// Delete removes the webhook with its delivery log.
func (uc *Webhooks) Delete(ctx context.Context, actor models.Actor, id int) error {
	const op = "usecase.Webhooks.Delete"

	if _, err := uc.get(ctx, op, actor, id); err != nil {
		return err
	}
	if err := uc.repos.Webhooks.DeleteWebhook(ctx, id); err != nil {
		return mapRepoErr(op, err)
	}

	return nil
}

// ListDeliveries returns a page of the webhook's delivery log, newest first.
func (uc *Webhooks) ListDeliveries(ctx context.Context, actor models.Actor, id int, p models.Pagination) (models.Page[models.WebhookDelivery], error) {
	const op = "usecase.Webhooks.ListDeliveries"

	if err := p.Validate(); err != nil {
		return models.Page[models.WebhookDelivery]{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if _, err := uc.get(ctx, op, actor, id); err != nil {
		return models.Page[models.WebhookDelivery]{}, err
	}

	page, err := uc.repos.Webhooks.GetDeliveriesByWebhookId(ctx, id, p)
	if err != nil {
		return page, mapRepoErr(op, err)
	}

	return page, nil
}

// SendTest delivers a synthetic "webhook.test" event to the webhook once
// (no retries) and returns the delivery with its outcome. An inactive
// webhook is tested too, so a disabled one can be checked before
// re-enabling it.
func (uc *Webhooks) SendTest(ctx context.Context, actor models.Actor, id int) (models.WebhookDelivery, error) {
	const op = "usecase.Webhooks.SendTest"

	w, err := uc.get(ctx, op, actor, id)
	if err != nil {
		return models.WebhookDelivery{}, err
	}

	eventID := uuid.NewString()
	data, err := json.Marshal(map[string]any{"webhook_id": w.ID, "message": "test delivery"})
	if err != nil {
		return models.WebhookDelivery{}, fmt.Errorf("%s: %w", op, err)
	}
	body, err := uc.payload(eventID, models.WebhookSubjectTest, data)
	if err != nil {
		return models.WebhookDelivery{}, fmt.Errorf("%s: %w", op, err)
	}

	d, _, err := uc.repos.Webhooks.AddDelivery(ctx, models.WebhookDelivery{
		WebhookID: w.ID,
		EventID:   eventID,
		Subject:   models.WebhookSubjectTest,
		Payload:   body,
	})
	if err != nil {
		return models.WebhookDelivery{}, mapRepoErr(op, err)
	}

	d, err = uc.attempt(ctx, w, d)
	if err != nil {
		return models.WebhookDelivery{}, fmt.Errorf("%s: %w", op, err)
	}

	return d, nil
}

// payload renders the JSON body of a delivery.
func (uc *Webhooks) payload(eventID, subject string, data json.RawMessage) (json.RawMessage, error) {
	body, err := json.Marshal(models.WebhookPayload{
		EventID:    eventID,
		Subject:    subject,
		OccurredAt: uc.now().UTC(),
		Data:       data,
	})
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	return body, nil
}

// Dispatch fans the event out to every active webhook subscribed to
// subject: a delivery row is created per webhook (idempotent on event id)
// and the first attempt is made right away; failed attempts are retried by
// RetryDue. Errors of individual deliveries are joined and returned after
// every webhook was tried.
func (uc *Webhooks) Dispatch(ctx context.Context, subject, eventID string, data json.RawMessage) error {
	const op = "usecase.Webhooks.Dispatch"

	if len(data) > MaxWebhookPayloadBytes {
		return fmt.Errorf("%s: %w (%d bytes, max %d)", op, ErrWebhookPayloadTooLarge, len(data), MaxWebhookPayloadBytes)
	}

	hooks, err := uc.repos.Webhooks.GetActiveWebhooksForEvent(ctx, subject)
	if err != nil {
		return mapRepoErr(op, err)
	}
	if len(hooks) == 0 {
		return nil
	}

	body, err := uc.payload(eventID, subject, data)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
		sem  = make(chan struct{}, webhookDispatchConcurrency)
	)
	for _, w := range hooks {
		d, created, err := uc.repos.Webhooks.AddDelivery(ctx, models.WebhookDelivery{
			WebhookID:     w.ID,
			EventID:       eventID,
			Subject:       subject,
			Payload:       body,
			NextAttemptAt: null.TimeFrom(uc.now().Add(webhookClaimLease)),
		})
		if err != nil {
			errs = append(errs, mapRepoErr(op, err))
			continue
		}
		if !created {
			uc.log.Debug("event already queued for webhook",
				slog.String("event_id", eventID), slog.Int("webhook_id", w.ID))
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := uc.attempt(ctx, w, d); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: webhook %d: %w", op, w.ID, err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	return errors.Join(errs...)
}

// RetryDue attempts every delivery whose retry time has come and returns
// how many were attempted.
func (uc *Webhooks) RetryDue(ctx context.Context, limit int) (int, error) {
	const op = "usecase.Webhooks.RetryDue"

	pending, err := uc.repos.Webhooks.ClaimDueDeliveries(ctx, uc.now(), webhookClaimLease, limit)
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	var errs []error
	for _, p := range pending {
		if _, err := uc.attempt(ctx, p.Webhook, p.Delivery); err != nil {
			errs = append(errs, fmt.Errorf("%s: delivery %d: %w", op, p.Delivery.ID, err))
		}
	}

	return len(pending), errors.Join(errs...)
}

// PruneDeliveries removes the delivery rows older than
// WebhookDeliveryRetention and returns how many were removed.
func (uc *Webhooks) PruneDeliveries(ctx context.Context) (int64, error) {
	const op = "usecase.Webhooks.PruneDeliveries"

	n, err := uc.repos.Webhooks.DeleteDeliveriesBefore(ctx, uc.now().Add(-WebhookDeliveryRetention))
	if err != nil {
		return 0, mapRepoErr(op, err)
	}

	return n, nil
}

// attempt performs one delivery attempt and records its outcome: success
// sets delivered_at; a failure schedules the next attempt per
// models.WebhookBackoff or, when the attempts are exhausted, deactivates
// the webhook and notifies its owner. Test deliveries are never retried.
// The returned delivery reflects the recorded state; the error covers only
// bookkeeping failures, not a failed HTTP attempt.
func (uc *Webhooks) attempt(ctx context.Context, w models.Webhook, d models.WebhookDelivery) (models.WebhookDelivery, error) {
	res := uc.deps.Sender.Send(ctx, webhooks.Request{
		WebhookID: w.ID,
		URL:       w.URL,
		Secret:    w.Secret,
		EventID:   d.EventID,
		Body:      d.Payload,
	})

	now := uc.now()
	result := models.WebhookAttemptResult{Attempt: d.Attempt + 1}
	if res.StatusCode != 0 {
		result.StatusCode = null.IntFrom(int64(res.StatusCode))
	}

	exhausted := false
	switch {
	case res.OK():
		result.DeliveredAt = null.TimeFrom(now)
	default:
		result.Error = null.StringFrom(truncate(res.Err.Error(), maxWebhookErrorLen))
		if d.Subject != models.WebhookSubjectTest {
			if delay, ok := models.WebhookBackoff(result.Attempt); ok {
				result.NextAttemptAt = null.TimeFrom(now.Add(delay))
			} else {
				exhausted = true
			}
		}
		uc.log.Warn("webhook delivery failed",
			slog.Int("webhook_id", w.ID),
			slog.Int64("delivery_id", d.ID),
			slog.Int("attempt", result.Attempt),
			slog.Int("status", res.StatusCode),
			slog.Bool("exhausted", exhausted),
			slogger.Err(res.Err),
		)
	}

	if err := uc.repos.Webhooks.RecordAttempt(ctx, d.ID, result); err != nil {
		return d, mapRepoErr("usecase.Webhooks.attempt", err)
	}
	d.Attempt = result.Attempt
	d.StatusCode = result.StatusCode
	d.Error = result.Error
	d.DeliveredAt = result.DeliveredAt
	d.NextAttemptAt = result.NextAttemptAt

	if exhausted {
		if err := uc.disable(ctx, w, result.Attempt); err != nil {
			return d, err
		}
	}

	return d, nil
}

// disable deactivates the webhook after exhausted attempts and tells the
// owner via an in-app notification (best effort).
func (uc *Webhooks) disable(ctx context.Context, w models.Webhook, attempts int) error {
	const op = "usecase.Webhooks.disable"

	if err := uc.repos.Webhooks.UpdateWebhook(ctx, w.ID, models.WebhookUpdate{Active: ptr(false)}); err != nil {
		return mapRepoErr(op, err)
	}
	uc.log.Warn("webhook deactivated after repeated failures", slog.Int("webhook_id", w.ID), slog.Int("attempts", attempts))

	if uc.deps.Notifications == nil {
		return nil
	}
	_, _, err := uc.deps.Notifications.Create(ctx, models.Notification{
		UserID: w.OwnerUserID,
		Type:   models.NotificationWebhookDisabled,
		Title:  "Вебхук отключён",
		Body:   fmt.Sprintf("Вебхук #%d (%s) отключён после %d неудачных попыток доставки", w.ID, w.URL, attempts),
	})
	if err != nil {
		uc.log.Warn("failed to notify webhook owner", slog.String("op", op), slogger.Err(err))
	}

	return nil
}

func ptr[T any](v T) *T { return &v }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
