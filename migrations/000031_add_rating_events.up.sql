-- Rating events: every change of users.rating is recorded here together
-- with the reason (see internal/models: RatingReason) and the mark/check
-- that caused it. users.rating stays the aggregate that the tasker reads;
-- UsersRepository.AddRatingEvent updates both in one statement.
CREATE TABLE IF NOT EXISTS rating_events (
    id BIGSERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    delta INTEGER NOT NULL,
    reason TEXT NOT NULL,
    mark_id INTEGER NULL,
    check_id INTEGER NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_rating_events_user FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE,
    CONSTRAINT fk_rating_events_mark FOREIGN KEY (mark_id) REFERENCES marks(mark_id) ON DELETE SET NULL,
    CONSTRAINT fk_rating_events_check FOREIGN KEY (check_id) REFERENCES checks(check_id) ON DELETE SET NULL
);

-- Per-user history, newest first.
CREATE INDEX IF NOT EXISTS idx_rating_events_user_id_created_at
    ON rating_events (user_id, created_at);

-- Leaderboard ordering.
CREATE INDEX IF NOT EXISTS idx_users_rating ON users (rating DESC, user_id ASC);

-- The daily check limit counts a user's checks by created_at.
CREATE INDEX IF NOT EXISTS idx_checks_user_id_created_at ON checks (user_id, created_at);
