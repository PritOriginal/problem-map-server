-- Tombstones of deleted marks so that offline clients can drop them on
-- incremental sync (GET /marks/changes). A row is written by DeleteMark in
-- the same transaction that removes the mark.
CREATE TABLE IF NOT EXISTS mark_tombstones (
    mark_id INTEGER PRIMARY KEY,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mark_tombstones_deleted_at ON mark_tombstones (deleted_at);
