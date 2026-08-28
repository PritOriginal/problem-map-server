CREATE INDEX IF NOT EXISTS idx_marks_user_id ON marks (user_id);
CREATE INDEX IF NOT EXISTS idx_marks_mark_status_id ON marks (mark_status_id);
CREATE INDEX IF NOT EXISTS idx_marks_type_mark_id ON marks (type_mark_id);

CREATE INDEX IF NOT EXISTS idx_checks_mark_id ON checks (mark_id);
CREATE INDEX IF NOT EXISTS idx_checks_user_id ON checks (user_id);
CREATE INDEX IF NOT EXISTS idx_checks_mark_status_history_id ON checks (mark_status_history_id);

CREATE INDEX IF NOT EXISTS idx_tasks_user_id ON tasks (user_id);
CREATE INDEX IF NOT EXISTS idx_tasks_mark_id ON tasks (mark_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status_id ON tasks (status_id);

-- idx_mark_status_history_mark_id already exists (see 000013).
CREATE INDEX IF NOT EXISTS idx_mark_status_history_prev_id ON mark_status_history (prev_id);

CREATE INDEX IF NOT EXISTS idx_users_home_point ON users USING GIST (home_point);
