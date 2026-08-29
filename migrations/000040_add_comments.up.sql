-- Comments on marks. A comment is either top-level or a reply to a
-- top-level one (parent_id, a single level of nesting is enforced by the
-- use case). Deleting is soft (deleted_at) so that replies to a removed
-- comment do not lose their parent; the removed body is not served.
CREATE TABLE IF NOT EXISTS mark_comments (
    comment_id SERIAL PRIMARY KEY,
    mark_id    INTEGER NOT NULL REFERENCES marks(mark_id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    body       TEXT NOT NULL CHECK (char_length(body) <= 2000),
    parent_id  INTEGER NULL REFERENCES mark_comments(comment_id) ON DELETE CASCADE,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The thread of a mark, oldest first (GET /marks/{id}/comments) and the
-- comments_count aggregate of a mark.
CREATE INDEX IF NOT EXISTS idx_mark_comments_mark_created ON mark_comments (mark_id, created_at);
-- Per-user daily limit and the duplicate check of POST /marks/{id}/comments.
CREATE INDEX IF NOT EXISTS idx_mark_comments_user_created ON mark_comments (user_id, created_at);
