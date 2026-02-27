ALTER TABLE mark_status_history DROP CONSTRAINT IF EXISTS fk_prev_mark_status_history;
ALTER TABLE mark_status_history DROP COLUMN IF EXISTS prev_id;

CREATE OR REPLACE FUNCTION log_mark_status_change()
RETURNS TRIGGER AS $$ 
BEGIN
    INSERT INTO mark_status_history (mark_id, old_mark_status_id, new_mark_status_id)
    VALUES (NEW.mark_id, OLD.mark_status_id, NEW.mark_status_id);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;