-- Before the constraint existed AddCheck verified uniqueness outside the
-- transaction, so duplicates may exist. Keep the earliest check per
-- (user, history entry) and drop the rest, otherwise ADD CONSTRAINT fails and
-- leaves the migration dirty.
DELETE FROM checks c
USING checks d
WHERE c.user_id = d.user_id
  AND c.mark_status_history_id = d.mark_status_history_id
  AND c.check_id > d.check_id;

ALTER TABLE checks ADD CONSTRAINT unique_check_per_history UNIQUE (user_id, mark_status_history_id);
