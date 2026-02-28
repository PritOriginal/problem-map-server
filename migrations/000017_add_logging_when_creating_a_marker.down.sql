DROP TRIGGER IF EXISTS insert_mark_status ON marks;

CREATE OR REPLACE FUNCTION log_mark_status_change()
RETURNS TRIGGER AS $$ 
BEGIN
    INSERT INTO mark_status_history (mark_id, old_mark_status_id, new_mark_status_id)
    VALUES (NEW.mark_id, OLD.mark_status_id, NEW.mark_status_id);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;