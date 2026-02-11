DROP TRIGGER IF EXISTS update_mark_status ON marks;

DROP FUNCTION IF EXISTS log_mark_status_change();

DROP INDEX IF EXISTS idx_mark_status_history_mark_id;
DROP INDEX IF EXISTS idx_mark_status_history_changed_at;

DROP TABLE IF EXISTS mark_status_history;