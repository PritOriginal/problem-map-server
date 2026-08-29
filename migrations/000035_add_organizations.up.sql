-- City services (organizations) that resolve confirmed marks.

-- Members of an organization get the 'service' role.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'moderator', 'admin', 'service'));

CREATE TABLE IF NOT EXISTS organizations (
    organization_id SERIAL PRIMARY KEY,
    name            VARCHAR(255) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A user belongs to at most one organization.
CREATE TABLE IF NOT EXISTS organization_members (
    organization_id INTEGER NOT NULL REFERENCES organizations(organization_id) ON DELETE CASCADE,
    user_id         INTEGER PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_organization_members_organization_id
    ON organization_members (organization_id);

-- admin_boundaries may have been re-created by the OSM importer without a
-- primary key; the FK below needs one.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'admin_boundaries'::regclass AND contype = 'p'
    ) THEN
        ALTER TABLE admin_boundaries ADD PRIMARY KEY (id);
    END IF;
END $$;

-- What an organization is responsible for: a mark type inside a boundary.
CREATE TABLE IF NOT EXISTS organization_responsibilities (
    id              SERIAL PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(organization_id) ON DELETE CASCADE,
    mark_type_id    INTEGER NOT NULL REFERENCES types_marks(type_mark_id),
    boundary_id     INTEGER NOT NULL REFERENCES admin_boundaries(id) ON DELETE CASCADE,
    CONSTRAINT uq_organization_responsibility UNIQUE (organization_id, mark_type_id, boundary_id)
);

CREATE INDEX IF NOT EXISTS idx_organization_responsibilities_type_boundary
    ON organization_responsibilities (mark_type_id, boundary_id);

-- Assignment of a mark to an organization, its SLA deadline and the time
-- the breach of that deadline was reported (NULL until reported; reset on
-- reassignment).
ALTER TABLE marks
    ADD COLUMN IF NOT EXISTS organization_id INTEGER NULL REFERENCES organizations(organization_id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS sla_due_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS sla_breached_at TIMESTAMPTZ NULL;

-- The organization's queue: its marks ordered by deadline.
CREATE INDEX IF NOT EXISTS idx_marks_organization_sla_due_at
    ON marks (organization_id, sla_due_at);

-- Time an organization has to resolve a mark of the type.
ALTER TABLE types_marks
    ADD COLUMN IF NOT EXISTS sla_hours INTEGER NOT NULL DEFAULT 72;

-- 'В работе': the organization started resolving the mark (child of
-- 'Подтверждённая', like the other post-confirmation stages).
INSERT INTO mark_statuses (mark_status_id, name, parent_id)
VALUES (7, 'В работе', 2)
ON CONFLICT (mark_status_id) DO NOTHING;

SELECT setval('mark_statuses_mark_status_id_seq', (SELECT MAX(mark_status_id) FROM mark_statuses));
