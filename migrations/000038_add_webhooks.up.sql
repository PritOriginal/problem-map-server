-- Outgoing webhooks: HTTP subscriptions to domain events (mark.*, task.*,
-- check.*) owned by a moderator/admin. The secret signs every delivery
-- (X-Signature: sha256=HMAC-SHA256(secret, "<X-Timestamp>." || body)).
CREATE TABLE IF NOT EXISTS webhooks (
    webhook_id    SERIAL PRIMARY KEY,
    owner_user_id INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    url           TEXT NOT NULL,
    secret        TEXT NOT NULL,
    events        TEXT[] NOT NULL,
    active        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhooks_owner ON webhooks (owner_user_id);
-- Event fan-out: active webhooks whose events contain the subject.
CREATE INDEX IF NOT EXISTS idx_webhooks_active_events ON webhooks USING GIN (events) WHERE active;

-- One delivery per (webhook, event); attempts update the row in place.
-- next_attempt_at IS NOT NULL marks a delivery waiting for a retry,
-- delivered_at IS NOT NULL a successful one. payload is the exact body
-- that was (and will be re-) sent, so retries stay byte-identical and the
-- signature the receiver saw can be verified later.
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    delivery_id     BIGSERIAL PRIMARY KEY,
    webhook_id      INTEGER NOT NULL REFERENCES webhooks(webhook_id) ON DELETE CASCADE,
    event_id        UUID NOT NULL,
    subject         TEXT NOT NULL,
    payload         JSONB NOT NULL,
    attempt         INTEGER NOT NULL DEFAULT 0,
    status_code     INTEGER NULL,
    error           TEXT NULL,
    delivered_at    TIMESTAMPTZ NULL,
    next_attempt_at TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_webhook_deliveries_webhook_event UNIQUE (webhook_id, event_id)
);

-- Delivery log per webhook, newest first.
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook
    ON webhook_deliveries (webhook_id, delivery_id DESC);
-- Retention: the notifier deletes deliveries older than 30 days by created_at.
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_created
    ON webhook_deliveries (created_at);
-- Retry scheduler: deliveries due for another attempt.
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_next_attempt
    ON webhook_deliveries (next_attempt_at) WHERE next_attempt_at IS NOT NULL;
