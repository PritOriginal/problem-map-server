DROP INDEX IF EXISTS idx_rating_events_mark_id;
DROP TABLE IF EXISTS user_badges;
DROP TABLE IF EXISTS badges;
ALTER TABLE users DROP COLUMN IF EXISTS created_at;
