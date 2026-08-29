package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/PritOriginal/problem-map-server/internal/models"
	"github.com/PritOriginal/problem-map-server/internal/repository"
	trmsqlx "github.com/avito-tech/go-transaction-manager/drivers/sqlx/v2"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// WebhooksRepository stores webhook subscriptions and their deliveries.
type WebhooksRepository struct {
	db     *sqlx.DB
	getter *trmsqlx.CtxGetter
}

func NewWebhooks(db *sqlx.DB, c *trmsqlx.CtxGetter) *WebhooksRepository {
	return &WebhooksRepository{
		db:     db,
		getter: c,
	}
}

const webhookColumns = "webhook_id, owner_user_id, url, secret, events, active, created_at"

// webhookRow is the scan target: text[] needs pq.StringArray.
type webhookRow struct {
	models.Webhook
	Events pq.StringArray `db:"events"`
}

func (r webhookRow) model() models.Webhook {
	w := r.Webhook
	w.Events = []string(r.Events)
	return w
}

func webhookModels(rows []webhookRow) []models.Webhook {
	out := make([]models.Webhook, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.model())
	}
	return out
}

// AddWebhook inserts w and returns its id.
func (r *WebhooksRepository) AddWebhook(ctx context.Context, w models.Webhook) (int64, error) {
	const op = "storage.postgres.AddWebhook"

	query := `
			INSERT INTO
				webhooks (owner_user_id, url, secret, events, active)
			VALUES
				($1, $2, $3, $4, $5)
			RETURNING webhook_id
			`
	var id int64
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &id, query, w.OwnerUserID, w.URL, w.Secret, pq.Array(w.Events), w.Active); err != nil {
		return 0, wrapPgError(op, err)
	}

	return id, nil
}

// GetWebhookById returns the webhook (repository.ErrNotFound when missing).
func (r *WebhooksRepository) GetWebhookById(ctx context.Context, id int) (models.Webhook, error) {
	const op = "storage.postgres.GetWebhookById"

	var row webhookRow
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.GetContext(ctx, &row, "SELECT "+webhookColumns+" FROM webhooks WHERE webhook_id = $1", id); err != nil {
		return models.Webhook{}, wrapPgError(op, err)
	}

	return row.model(), nil
}

// GetWebhooksByOwner lists the user's webhooks, oldest first.
func (r *WebhooksRepository) GetWebhooksByOwner(ctx context.Context, ownerUserID int) ([]models.Webhook, error) {
	const op = "storage.postgres.GetWebhooksByOwner"

	rows := []webhookRow{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	query := "SELECT " + webhookColumns + " FROM webhooks WHERE owner_user_id = $1 ORDER BY webhook_id"
	if err := tr.SelectContext(ctx, &rows, query, ownerUserID); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return webhookModels(rows), nil
}

// GetActiveWebhooksForEvent returns the active webhooks subscribed to
// subject: exactly, via the "<prefix>.*" pattern or via "*".
func (r *WebhooksRepository) GetActiveWebhooksForEvent(ctx context.Context, subject string) ([]models.Webhook, error) {
	const op = "storage.postgres.GetActiveWebhooksForEvent"

	rows := []webhookRow{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	// The array overlap (&&) uses the GIN index on events.
	query := `
		SELECT ` + webhookColumns + `
		FROM webhooks
		WHERE active AND events && $1
		ORDER BY webhook_id
		`
	patterns := pq.Array([]string{subject, models.WebhookEventAll, subjectPrefixPattern(subject)})
	if err := tr.SelectContext(ctx, &rows, query, patterns); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return webhookModels(rows), nil
}

// subjectPrefixPattern returns "mark.*" for "mark.status_changed".
func subjectPrefixPattern(subject string) string {
	for i := 0; i < len(subject); i++ {
		if subject[i] == '.' {
			return subject[:i] + ".*"
		}
	}
	return subject + ".*"
}

// UpdateWebhook changes the given fields (repository.ErrNotFound when missing).
func (r *WebhooksRepository) UpdateWebhook(ctx context.Context, id int, upd models.WebhookUpdate) error {
	const op = "storage.postgres.UpdateWebhook"

	query := `
		UPDATE webhooks SET
			active = COALESCE($2, active),
			events = COALESCE($3, events)
		WHERE webhook_id = $1
		`
	var events any
	if upd.Events != nil {
		events = pq.Array(upd.Events)
	}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, query, id, upd.Active, events)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// DeleteWebhook removes the webhook and its deliveries (ON DELETE CASCADE).
func (r *WebhooksRepository) DeleteWebhook(ctx context.Context, id int) error {
	const op = "storage.postgres.DeleteWebhook"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, "DELETE FROM webhooks WHERE webhook_id = $1", id)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

const deliveryColumns = "delivery_id, webhook_id, event_id, subject, payload, attempt, status_code, error, delivered_at, next_attempt_at, created_at"

// AddDelivery inserts a pending delivery (attempt 0) and returns it. A
// delivery of the same event to the same webhook already exists: created
// is false and the stored row is returned, which keeps event redelivery
// idempotent.
func (r *WebhooksRepository) AddDelivery(ctx context.Context, d models.WebhookDelivery) (models.WebhookDelivery, bool, error) {
	const op = "storage.postgres.AddDelivery"

	query := `
			INSERT INTO
				webhook_deliveries (webhook_id, event_id, subject, payload, next_attempt_at)
			VALUES
				($1, $2, $3, $4, $5)
			ON CONFLICT (webhook_id, event_id) DO NOTHING
			RETURNING ` + deliveryColumns
	var out models.WebhookDelivery
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	err := tr.GetContext(ctx, &out, query, d.WebhookID, d.EventID, d.Subject, []byte(d.Payload), d.NextAttemptAt)
	switch {
	case err == nil:
		return out, true, nil
	case errors.Is(err, sql.ErrNoRows):
		existing, err := r.getDelivery(ctx, "webhook_id = $1 AND event_id = $2", d.WebhookID, d.EventID)
		if err != nil {
			return models.WebhookDelivery{}, false, wrapPgError(op, err)
		}
		return existing, false, nil
	default:
		return models.WebhookDelivery{}, false, wrapPgError(op, err)
	}
}

// GetDeliveryById returns the delivery (repository.ErrNotFound when missing).
func (r *WebhooksRepository) GetDeliveryById(ctx context.Context, id int64) (models.WebhookDelivery, error) {
	const op = "storage.postgres.GetDeliveryById"

	d, err := r.getDelivery(ctx, "delivery_id = $1", id)
	if err != nil {
		return d, wrapPgError(op, err)
	}
	return d, nil
}

func (r *WebhooksRepository) getDelivery(ctx context.Context, where string, args ...any) (models.WebhookDelivery, error) {
	var d models.WebhookDelivery
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	err := tr.GetContext(ctx, &d, "SELECT "+deliveryColumns+" FROM webhook_deliveries WHERE "+where, args...)
	return d, err
}

// GetDeliveriesByWebhookId returns a page of the webhook's deliveries,
// newest first.
func (r *WebhooksRepository) GetDeliveriesByWebhookId(ctx context.Context, webhookID int, p models.Pagination) (models.Page[models.WebhookDelivery], error) {
	const op = "storage.postgres.GetDeliveriesByWebhookId"

	q := newListQuery(deliveryColumns, "webhook_deliveries").
		Where("webhook_id = ?", webhookID).
		OrderBy("delivery_id DESC").
		Paginate(p)

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	page, err := selectPage[models.WebhookDelivery](ctx, tr, q)
	if err != nil {
		return page, fmt.Errorf("%s: %w", op, err)
	}

	return page, nil
}

// ClaimDueDeliveries returns up to limit deliveries whose next_attempt_at
// has passed, together with their webhooks, and pushes next_attempt_at by
// lease so a concurrent worker (or a crashed attempt) does not pick them
// up again meanwhile. Deliveries of inactive webhooks are left alone.
func (r *WebhooksRepository) ClaimDueDeliveries(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.PendingWebhookDelivery, error) {
	const op = "storage.postgres.ClaimDueDeliveries"

	query := `
		WITH due AS (
			SELECT d.delivery_id
			FROM webhook_deliveries d
			JOIN webhooks w ON w.webhook_id = d.webhook_id
			WHERE d.next_attempt_at IS NOT NULL AND d.next_attempt_at <= $1 AND w.active
			ORDER BY d.next_attempt_at
			LIMIT $3
			FOR UPDATE OF d SKIP LOCKED
		),
		claimed AS (
			UPDATE webhook_deliveries d
			SET next_attempt_at = $1::timestamptz + make_interval(secs => $2)
			FROM due
			WHERE d.delivery_id = due.delivery_id
			RETURNING d.*
		)
		SELECT
			c.delivery_id, c.webhook_id, c.event_id, c.subject, c.payload, c.attempt, c.status_code, c.error,
			c.delivered_at, c.next_attempt_at, c.created_at,
			w.webhook_id AS "webhook.webhook_id", w.owner_user_id AS "webhook.owner_user_id", w.url AS "webhook.url",
			w.secret AS "webhook.secret", w.events AS "webhook.events", w.active AS "webhook.active",
			w.created_at AS "webhook.created_at"
		FROM claimed c
		JOIN webhooks w ON w.webhook_id = c.webhook_id
		ORDER BY c.delivery_id
		`
	type row struct {
		models.WebhookDelivery
		Webhook webhookRow `db:"webhook"`
	}
	rows := []row{}
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	if err := tr.SelectContext(ctx, &rows, query, now, lease.Seconds(), limit); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	out := make([]models.PendingWebhookDelivery, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.PendingWebhookDelivery{Delivery: r.WebhookDelivery, Webhook: r.Webhook.model()})
	}
	return out, nil
}

// RecordAttempt stores the outcome of an attempt on the delivery.
func (r *WebhooksRepository) RecordAttempt(ctx context.Context, deliveryID int64, res models.WebhookAttemptResult) error {
	const op = "storage.postgres.RecordAttempt"

	query := `
		UPDATE webhook_deliveries SET
			attempt = $2,
			status_code = $3,
			error = $4,
			delivered_at = $5,
			next_attempt_at = $6
		WHERE delivery_id = $1
		`
	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	result, err := tr.ExecContext(ctx, query, deliveryID, res.Attempt, res.StatusCode, res.Error, res.DeliveredAt, res.NextAttemptAt)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// DeleteDeliveriesBefore removes the deliveries created before the given
// time (retention of the delivery log) and returns how many were removed.
func (r *WebhooksRepository) DeleteDeliveriesBefore(ctx context.Context, before time.Time) (int64, error) {
	const op = "storage.postgres.DeleteDeliveriesBefore"

	tr := r.getter.DefaultTrOrDB(ctx, r.db)
	res, err := tr.ExecContext(ctx, "DELETE FROM webhook_deliveries WHERE created_at < $1", before)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return n, nil
}
