-- Marks in progress go back to confirmed (the trigger logs 7 -> 2), then
-- every trace of status 7 is removed in FK order: checks, history, status.
UPDATE marks SET mark_status_id = 2 WHERE mark_status_id = 7;
DELETE FROM checks WHERE mark_status_id = 7 OR mark_status_history_id IN (
    SELECT id FROM mark_status_history WHERE old_mark_status_id = 7 OR new_mark_status_id = 7
);
UPDATE mark_status_history SET prev_id = NULL WHERE prev_id IN (
    SELECT id FROM mark_status_history WHERE old_mark_status_id = 7 OR new_mark_status_id = 7
);
DELETE FROM mark_status_history WHERE old_mark_status_id = 7 OR new_mark_status_id = 7;
DELETE FROM mark_statuses WHERE mark_status_id = 7;
SELECT setval('mark_statuses_mark_status_id_seq', (SELECT MAX(mark_status_id) FROM mark_statuses));

ALTER TABLE types_marks DROP COLUMN IF EXISTS sla_hours;

DROP INDEX IF EXISTS idx_marks_organization_sla_due_at;
ALTER TABLE marks
    DROP COLUMN IF EXISTS sla_breached_at,
    DROP COLUMN IF EXISTS sla_due_at,
    DROP COLUMN IF EXISTS organization_id;

DROP INDEX IF EXISTS idx_organization_responsibilities_type_boundary;
DROP TABLE IF EXISTS organization_responsibilities;

DROP INDEX IF EXISTS idx_organization_members_organization_id;
DROP TABLE IF EXISTS organization_members;
DROP TABLE IF EXISTS organizations;

UPDATE users SET role = 'user' WHERE role = 'service';
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'moderator', 'admin'));
