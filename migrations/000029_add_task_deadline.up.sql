-- Task deadlines: the tasker sets due_at when issuing a task; tasks still
-- issued after due_at are moved to the new «Просрочено» status.
ALTER TABLE tasks ADD COLUMN due_at TIMESTAMPTZ NULL;

-- status_id = 3 (see internal/models: OverdueStatus). The id is fixed
-- explicitly so that the constant in code never drifts from the data.
INSERT INTO task_statuses (status_id, name) VALUES (3, 'Просрочено')
ON CONFLICT (status_id) DO NOTHING;
SELECT setval('task_statuses_status_id_seq', (SELECT MAX(status_id) FROM task_statuses));

-- ExpireOverdue scans tasks by (status_id, due_at).
CREATE INDEX IF NOT EXISTS idx_tasks_status_id_due_at ON tasks (status_id, due_at);

-- A user can hold at most one issued task per mark. This is the safety net
-- against two overlapping tasker runs; the tasker itself also skips
-- (user, mark) pairs that already have a task in any status.
CREATE UNIQUE INDEX IF NOT EXISTS uq_tasks_issued_user_mark
    ON tasks (user_id, mark_id) WHERE status_id = 1;
