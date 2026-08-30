DROP INDEX IF EXISTS idx_webhook_deliveries_next_attempt;
DROP INDEX IF EXISTS idx_webhook_deliveries_webhook;
DROP TABLE IF EXISTS webhook_deliveries;

DROP INDEX IF EXISTS idx_webhooks_active_events;
DROP INDEX IF EXISTS idx_webhooks_owner;
DROP TABLE IF EXISTS webhooks;
