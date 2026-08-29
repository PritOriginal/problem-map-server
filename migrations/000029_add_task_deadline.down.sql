DROP INDEX IF EXISTS uq_tasks_issued_user_mark;
DROP INDEX IF EXISTS idx_tasks_status_id_due_at;

-- Return overdue tasks to «Выдано» before removing the status.
UPDATE tasks SET status_id = 1 WHERE status_id = 3;
DELETE FROM task_statuses WHERE status_id = 3;

ALTER TABLE tasks DROP COLUMN IF EXISTS due_at;
