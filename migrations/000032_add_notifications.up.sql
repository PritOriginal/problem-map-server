-- In-app notifications produced by cmd/notifier from domain events.
-- event_id + user_id make the consumer idempotent: redelivery of the same
-- event never creates a second notification for the same user.
CREATE TABLE IF NOT EXISTS notifications (
    notification_id SERIAL PRIMARY KEY,
    user_id         INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    event_id        UUID NOT NULL,
    type            TEXT NOT NULL,
    mark_id         INTEGER NULL REFERENCES marks(mark_id) ON DELETE CASCADE,
    task_id         INTEGER NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    read_at         TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_notifications_event_user UNIQUE (event_id, user_id)
);

-- Listing (all / unread) and unread counting per user.
CREATE INDEX IF NOT EXISTS idx_notifications_user_read_created
    ON notifications (user_id, read_at, created_at DESC);

-- Push tokens registered by clients. A token belongs to one user at a time.
CREATE TABLE IF NOT EXISTS user_devices (
    device_id  SERIAL PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    platform   TEXT NOT NULL CHECK (platform IN ('android', 'ios', 'web')),
    token      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_devices_user_id ON user_devices (user_id);
