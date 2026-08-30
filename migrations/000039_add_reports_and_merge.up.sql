-- Moderation: user reports on marks/checks/comments, hidden marks and the
-- merge of duplicate marks.

-- One report per (reporter, target): a repeated report is a conflict.
-- target_id is not a foreign key because the target lives in one of
-- several tables (comments may not exist yet).
CREATE TABLE IF NOT EXISTS reports (
    report_id   SERIAL PRIMARY KEY,
    reporter_id INTEGER NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    target_type TEXT NOT NULL CHECK (target_type IN ('mark', 'check', 'comment')),
    target_id   INTEGER NOT NULL CHECK (target_id > 0),
    reason      TEXT NOT NULL CHECK (reason IN ('spam', 'offensive', 'wrong_place', 'duplicate', 'other')),
    comment     TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved', 'dismissed')),
    resolved_by INTEGER NULL REFERENCES users(user_id) ON DELETE SET NULL,
    resolved_at TIMESTAMPTZ NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reports_reporter_target UNIQUE (reporter_id, target_type, target_id)
);

-- Moderation queue: open reports, oldest first.
CREATE INDEX IF NOT EXISTS idx_reports_status_created ON reports (status, created_at, report_id);
-- Auto-hide threshold: open reports per target.
CREATE INDEX IF NOT EXISTS idx_reports_open_target ON reports (target_type, target_id) WHERE status = 'open';
-- Daily limit and "my reports".
CREATE INDEX IF NOT EXISTS idx_reports_reporter_created ON reports (reporter_id, created_at DESC);

-- hidden: the mark is excluded from public lists (auto-hidden by reports
-- or by a moderator); merged_into_id: the mark was merged into another
-- one as a duplicate.
ALTER TABLE marks
    ADD COLUMN IF NOT EXISTS hidden BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS merged_into_id INTEGER NULL REFERENCES marks(mark_id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_marks_merged_into_id ON marks (merged_into_id) WHERE merged_into_id IS NOT NULL;

-- 'Дубликат': the mark was merged into another one (terminal status).
INSERT INTO mark_statuses (mark_status_id, name, parent_id, code)
VALUES (8, 'Дубликат', NULL, 'duplicate')
ON CONFLICT (mark_status_id) DO NOTHING;

INSERT INTO translations (entity, entity_id, lang, name)
VALUES ('mark_status', 8, 'ru', 'Дубликат'), ('mark_status', 8, 'en', 'Duplicate')
ON CONFLICT (entity, entity_id, lang) DO NOTHING;

SELECT setval('mark_statuses_mark_status_id_seq', (SELECT MAX(mark_status_id) FROM mark_statuses));
