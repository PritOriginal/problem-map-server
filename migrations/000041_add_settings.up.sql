-- Runtime settings editable by administrators (GET/PUT /admin/settings).
-- One row per key; the value is a JSON document, e.g. key 'runtime' holds
-- the voting threshold, rating deltas and tasker limits. Every change is
-- recorded in settings_history by the trigger below.
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_by INTEGER NULL REFERENCES users(user_id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE settings_history (
    id         BIGSERIAL PRIMARY KEY,
    key        TEXT NOT NULL,
    old        JSONB NULL,
    new        JSONB NOT NULL,
    updated_by INTEGER NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX settings_history_updated_at_idx ON settings_history (updated_at DESC, id DESC);

CREATE OR REPLACE FUNCTION settings_record_history() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.value = NEW.value THEN
        RETURN NEW;
    END IF;
    INSERT INTO settings_history (key, old, new, updated_by, updated_at)
    VALUES (NEW.key, CASE WHEN TG_OP = 'UPDATE' THEN OLD.value END, NEW.value, NEW.updated_by, NEW.updated_at);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER settings_history_trigger
AFTER INSERT OR UPDATE ON settings
FOR EACH ROW EXECUTE FUNCTION settings_record_history();

-- Presentation and lifecycle attributes of mark types, managed via
-- /admin/mark-types. Inactive types are hidden from GET /marks/types but
-- keep their existing marks.
ALTER TABLE types_marks
    ADD COLUMN icon       TEXT NULL,
    ADD COLUMN color      TEXT NULL,
    ADD COLUMN active     BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
