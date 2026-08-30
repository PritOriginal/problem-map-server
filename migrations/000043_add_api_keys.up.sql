-- API keys for the open-data endpoints: read-only access to the public GET
-- routes with a per-key rate limit. Only the SHA-256 of the key is stored;
-- prefix keeps the displayable head of the key ("pm_live_" + 8 hex chars)
-- so a user can tell their keys apart. scopes reserves "write" for later,
-- the server currently honours "read" only.
CREATE TABLE IF NOT EXISTS api_keys (
    api_key_id         SERIAL PRIMARY KEY,
    owner_user_id      INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    name               TEXT NOT NULL,
    key_hash           TEXT NOT NULL UNIQUE,
    prefix             TEXT NOT NULL,
    scopes             TEXT[] NOT NULL DEFAULT '{read}',
    rate_limit_per_min INTEGER NOT NULL DEFAULT 600,
    active             BOOLEAN NOT NULL DEFAULT TRUE,
    last_used_at       TIMESTAMPTZ NULL,
    expires_at         TIMESTAMPTZ NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_owner ON api_keys (owner_user_id);
