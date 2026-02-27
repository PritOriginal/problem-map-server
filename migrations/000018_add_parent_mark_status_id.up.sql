ALTER TABLE mark_statuses ADD COLUMN parent_id INTEGER;
ALTER TABLE mark_statuses ADD CONSTRAINT fk_parent_mark_status FOREIGN KEY (parent_id) REFERENCES mark_statuses (mark_status_id);
UPDATE mark_statuses SET parent_id = 1 WHERE name = 'Подтверждённая';