ALTER TABLE types_marks
    DROP COLUMN IF EXISTS sort_order,
    DROP COLUMN IF EXISTS active,
    DROP COLUMN IF EXISTS color,
    DROP COLUMN IF EXISTS icon;

DROP TRIGGER IF EXISTS settings_history_trigger ON settings;
DROP FUNCTION IF EXISTS settings_record_history();
DROP TABLE IF EXISTS settings_history;
DROP TABLE IF EXISTS settings;
