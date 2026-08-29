-- Revert the coordinate swap for exactly the rows logged in marks_coord_fix_log.

UPDATE marks m
SET geom = ST_SetSRID(ST_MakePoint(ST_Y(m.geom), ST_X(m.geom)), 4326)
FROM marks_coord_fix_log l
WHERE l.mark_id = m.mark_id;

DROP TABLE IF EXISTS marks_coord_fix_log;
