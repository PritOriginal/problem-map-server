CREATE INDEX idx_marks_geom ON marks USING GIST (geom);
CREATE INDEX idx_admin_boundaries_geom ON admin_boundaries USING GIST (geom);
CREATE INDEX idx_admin_boundaries_level ON admin_boundaries (admin_level);