-- Composite indexes matching the ORDER BY of the list endpoints so that a
-- LIMIT/OFFSET page is read in index order instead of sorting the table.

-- GET /marks (created_at DESC, mark_id DESC) and its updated_at variant.
CREATE INDEX IF NOT EXISTS idx_marks_created_at_desc ON marks (created_at DESC, mark_id DESC);
CREATE INDEX IF NOT EXISTS idx_marks_updated_at_desc ON marks (updated_at DESC, mark_id DESC);
-- GET /marks filtered by status, newest first.
CREATE INDEX IF NOT EXISTS idx_marks_status_created_at_desc ON marks (mark_status_id, created_at DESC, mark_id DESC);

-- GET /tasks/user/{id}: a user's tasks, newest first.
CREATE INDEX IF NOT EXISTS idx_tasks_user_id_created_at_desc ON tasks (user_id, created_at DESC);

-- GET /checks/mark/{id}: a mark's checks, newest first.
CREATE INDEX IF NOT EXISTS idx_checks_mark_id_created_at_desc ON checks (mark_id, created_at DESC);
