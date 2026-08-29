DROP TABLE IF EXISTS translations;

ALTER TABLE task_statuses DROP CONSTRAINT IF EXISTS task_statuses_code_key;
ALTER TABLE task_statuses DROP COLUMN IF EXISTS code;

ALTER TABLE mark_statuses DROP CONSTRAINT IF EXISTS mark_statuses_code_key;
ALTER TABLE mark_statuses DROP COLUMN IF EXISTS code;

ALTER TABLE types_marks DROP CONSTRAINT IF EXISTS types_marks_code_key;
ALTER TABLE types_marks DROP COLUMN IF EXISTS code;
