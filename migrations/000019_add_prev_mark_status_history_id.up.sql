ALTER TABLE mark_status_history ADD COLUMN prev_id INTEGER;
ALTER TABLE mark_status_history ADD CONSTRAINT fk_prev_mark_status_history FOREIGN KEY (prev_id) REFERENCES mark_status_history (id);

WITH ordered_records AS (
  SELECT 
    id,
    mark_id,
    changed_at,
    LAG(id) OVER (PARTITION BY mark_id ORDER BY changed_at, id) AS prev_record_id
  FROM mark_status_history
)
UPDATE mark_status_history AS t
SET prev_id = o.prev_record_id
FROM ordered_records AS o
WHERE t.id = o.id;

CREATE OR REPLACE FUNCTION log_mark_status_change()
RETURNS TRIGGER AS $$ 
BEGIN
    INSERT INTO mark_status_history (mark_id, old_mark_status_id, new_mark_status_id, prev_id)
    VALUES (NEW.mark_id, OLD.mark_status_id, NEW.mark_status_id, (
        SELECT id 
        FROM mark_status_history 
        WHERE mark_id = NEW.mark_id AND new_mark_status_id = OLD.mark_status_id
        ORDER BY changed_at DESC
        LIMIT 1
        )
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql; 