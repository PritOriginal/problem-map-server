ALTER TABLE checks ADD COLUMN mark_status_id INTEGER;
ALTER TABLE checks ADD COLUMN mark_status_history_id INTEGER;
UPDATE checks SET mark_status_id = 1;
ALTER TABLE checks ADD CONSTRAINT fk_checks_mark_status FOREIGN KEY (mark_status_id) REFERENCES mark_statuses (mark_status_id);
ALTER TABLE checks ADD CONSTRAINT fk_checks_mark_status_history FOREIGN KEY (mark_status_history_id) REFERENCES mark_status_history (id);