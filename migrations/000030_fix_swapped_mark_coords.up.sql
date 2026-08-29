-- Fix marks whose coordinates were stored as (latitude, longitude) instead of
-- (longitude, latitude). Old mobile clients sent lat/lon and the REST handler
-- built the point as Point(lat, lon), so X ended up ~52.7 instead of ~41.4.
--
-- A mark is considered swapped when it is NOT inside any admin boundary as is,
-- but IS inside some admin boundary after swapping X and Y.
--
-- NOTE: if admin_boundaries is empty, nothing matches and this migration is a
-- no-op. In that case the fix can be applied manually with the heuristic
-- "ST_X(geom) > ST_Y(geom)" (for the Tambov region longitude < latitude).
--
-- Affected ids are logged into marks_coord_fix_log so the down migration can
-- revert exactly these rows.

CREATE TABLE IF NOT EXISTS marks_coord_fix_log (
    mark_id  INTEGER PRIMARY KEY,
    fixed_at TIMESTAMP NOT NULL DEFAULT NOW()
);

INSERT INTO marks_coord_fix_log (mark_id)
SELECT m.mark_id
FROM marks m
WHERE NOT EXISTS (
        SELECT 1 FROM admin_boundaries b WHERE ST_Contains(b.geom, m.geom)
    )
  AND EXISTS (
        SELECT 1 FROM admin_boundaries b
        WHERE ST_Contains(b.geom, ST_SetSRID(ST_MakePoint(ST_Y(m.geom), ST_X(m.geom)), 4326))
    )
ON CONFLICT (mark_id) DO NOTHING;

UPDATE marks m
SET geom = ST_SetSRID(ST_MakePoint(ST_Y(m.geom), ST_X(m.geom)), 4326)
FROM marks_coord_fix_log l
WHERE l.mark_id = m.mark_id;
