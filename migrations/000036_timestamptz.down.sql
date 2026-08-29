-- Revert to TIMESTAMP (without zone) keeping the UTC wall-clock time.

ALTER TABLE marks
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE checks
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE tasks
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE mark_status_history
    ALTER COLUMN changed_at TYPE TIMESTAMP USING changed_at AT TIME ZONE 'UTC';

ALTER TABLE mark_followers
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';

ALTER TABLE marks_coord_fix_log
    ALTER COLUMN fixed_at TYPE TIMESTAMP USING fixed_at AT TIME ZONE 'UTC';
