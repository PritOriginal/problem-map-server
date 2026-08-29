CREATE TABLE IF NOT EXISTS mark_followers (
    user_id    INTEGER   NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    mark_id    INTEGER   NOT NULL REFERENCES marks(mark_id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, mark_id)
);

CREATE INDEX IF NOT EXISTS idx_mark_followers_mark_id ON mark_followers (mark_id);

-- Authors follow their own marks: backfill existing marks so that
-- followers_count / is_following are consistent for old data.
INSERT INTO mark_followers (user_id, mark_id, created_at)
SELECT user_id, mark_id, created_at FROM marks WHERE user_id IS NOT NULL
ON CONFLICT DO NOTHING;
