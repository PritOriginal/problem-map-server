-- Merged marks lose their status: they become closed, and the rows that
-- reference the status are rewritten so that the status row can be removed
-- (the history chain is kept intact).
UPDATE marks SET mark_status_id = 5 WHERE mark_status_id = 8;
UPDATE mark_status_history SET old_mark_status_id = NULL WHERE old_mark_status_id = 8;
UPDATE mark_status_history SET new_mark_status_id = 5 WHERE new_mark_status_id = 8;
UPDATE checks SET mark_status_id = 5 WHERE mark_status_id = 8;

DELETE FROM translations WHERE entity = 'mark_status' AND entity_id = 8;
DELETE FROM mark_statuses WHERE mark_status_id = 8;
SELECT setval('mark_statuses_mark_status_id_seq', (SELECT MAX(mark_status_id) FROM mark_statuses));

DROP INDEX IF EXISTS idx_marks_merged_into_id;
ALTER TABLE marks
    DROP COLUMN IF EXISTS merged_into_id,
    DROP COLUMN IF EXISTS hidden;

DROP INDEX IF EXISTS idx_reports_reporter_created;
DROP INDEX IF EXISTS idx_reports_open_target;
DROP INDEX IF EXISTS idx_reports_status_created;
DROP TABLE IF EXISTS reports;
