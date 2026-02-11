CREATE TABLE mark_status_history (
    id SERIAL PRIMARY KEY,
    mark_id INTEGER,
    old_mark_status_id INTEGER,
    new_mark_status_id INTEGER,
    changed_at TIMESTAMP DEFAULT NOW(),
    CONSTRAINT fk_old_mark_status FOREIGN KEY (old_mark_status_id) REFERENCES mark_statuses(mark_status_id),
    CONSTRAINT fk_new_mark_status FOREIGN KEY (new_mark_status_id) REFERENCES mark_statuses(mark_status_id)
);

CREATE OR REPLACE FUNCTION log_mark_status_change()
RETURNS TRIGGER AS $$ 
BEGIN
    INSERT INTO mark_status_history (mark_id, old_mark_status_id, new_mark_status_id)
    VALUES (NEW.mark_id, OLD.mark_status_id, NEW.mark_status_id);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_mark_status 
AFTER UPDATE ON marks 
FOR EACH ROW 
WHEN (OLD.mark_status_id IS DISTINCT FROM NEW.mark_status_id) 
EXECUTE FUNCTION log_mark_status_change();

CREATE INDEX idx_mark_status_history_mark_id 
ON mark_status_history(mark_id);

CREATE INDEX idx_mark_status_history_changed_at 
ON mark_status_history(changed_at);