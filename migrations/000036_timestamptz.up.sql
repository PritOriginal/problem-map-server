-- Store all event timestamps as TIMESTAMPTZ. The old TIMESTAMP columns were
-- always written in UTC (the application binds UTC values and the server ran
-- with timezone=UTC), so the stored wall-clock time is reinterpreted as UTC:
-- the instant does not change, only the type does.

ALTER TABLE marks
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE checks
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE tasks
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

ALTER TABLE mark_status_history
    ALTER COLUMN changed_at TYPE TIMESTAMPTZ USING changed_at AT TIME ZONE 'UTC';

ALTER TABLE mark_followers
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

ALTER TABLE marks_coord_fix_log
    ALTER COLUMN fixed_at TYPE TIMESTAMPTZ USING fixed_at AT TIME ZONE 'UTC';
