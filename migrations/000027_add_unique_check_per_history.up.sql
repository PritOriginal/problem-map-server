ALTER TABLE checks ADD CONSTRAINT unique_check_per_history UNIQUE (user_id, mark_status_history_id);
