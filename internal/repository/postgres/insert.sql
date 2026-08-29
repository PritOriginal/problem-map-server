INSERT INTO districts (name, geom)
	SELECT name, ST_Transform(way, 4326) 
	FROM planet_osm_polygon;


-- Since migration 000037 every dictionary row needs a stable `code`
-- (NOT NULL, UNIQUE) and its localised names live in `translations`.
INSERT INTO
	types_marks (name, code)
VALUES
	('Мусор', 'garbage'), ('Инфраструктура', 'infrastructure');

INSERT INTO translations (entity, entity_id, lang, name)
SELECT 'mark_type', type_mark_id, 'ru', name FROM types_marks
ON CONFLICT DO NOTHING;
INSERT INTO translations (entity, entity_id, lang, name)
SELECT 'mark_type', type_mark_id, 'en', CASE code WHEN 'garbage' THEN 'Garbage' WHEN 'infrastructure' THEN 'Infrastructure' END
FROM types_marks WHERE code IN ('garbage', 'infrastructure')
ON CONFLICT DO NOTHING;

INSERT INTO
	users (name, login, password_hash, home_point, rating) 
VALUES
	('Степан', 'Prit', 'qwer', ST_SetSRID(ST_MakePoint(41.400636, 52.699922), 4326), 0);

INSERT INTO 
	marks (name, geom, type_mark_id, user_id, district_id, number_votes, number_checks) 
VALUES
	('Свалка', ST_SetSRID(ST_MakePoint(41.402893, 52.700111), 4326), 1, 1, 2, 0, 0),
	('Ремонт труб', ST_SetSRID(ST_MakePoint(41.463077, 52.718319), 4326), 2, 1, 1, 0, 0);


SELECT (dp.gdump).path, (dp.gdump).geom FROM(
	SELECT ST_DumpPoints(points) AS gdump FROM (
		SELECT ST_GeneratePoints(s.geom, floor(random() * 10 + 1)::int, 1996) AS points
		FROM (
			SELECT way AS geom FROM planet_osm_polygon WHERE admin_level = '9'
		) AS s
	) AS p
) as dp;



INSERT INTO 
	marks (name, geom, type_mark_id, user_id, district_id)
SELECT 'test', (ST_DumpPoints(points)).geom, floor(random() * 2 + 1)::int, 1, p.district_id
FROM (
	SELECT s.district_id, s.geom, ST_GeneratePoints(s.geom, floor(random() * 20 + 1)::int, 1996) AS points
	FROM (
		SELECT district_id, geom FROM districts
	) AS s
) AS p;


SELECT
    b.id AS boundary_id,
    b.name AS boundary_name,
	COUNT(*) FILTER (WHERE m.mark_status_id = 1) AS unconfirmed_count,
    COUNT(*) FILTER (WHERE m.mark_status_id IN (2,4)) AS confirmed_count,
	COUNT(*) FILTER (WHERE m.mark_status_id = 3) AS under_review_count,
	COUNT(*) FILTER (WHERE m.mark_status_id = 5) AS closed_count
FROM
    admin_boundaries b
LEFT JOIN
    marks m ON ST_Contains(b.geom, m.geom)	
GROUP BY
    b.id, b.name
ORDER BY
    b.id;
	
-- BEGIN;
-- INSERT INTO districts (name, geom)
-- 	SELECT name, ST_Transform(way, 4326) 
-- 	FROM planet_osm_polygon 
-- 	WHERE boundary = 'administrative';

-- INSERT INTO 
-- 	types_marks (name)
-- VALUES
-- 	('Мусор'), ('Инфраструктура');

-- INSERT INTO 
-- 	users (name, login, password_hash, home_point, rating) 
-- VALUES
-- 	('Степан', 'Prit', 'qwer', ST_SetSRID(ST_MakePoint(41.400636, 52.699922), 4326), 0);

-- INSERT INTO 
-- 	marks (name, geom, type_mark_id, user_id, district_id, number_votes, number_checks) 
-- VALUES
-- 	('Свалка', ST_SetSRID(ST_MakePoint(41.402893, 52.700111), 4326), 1, 1, 2, 0, 0),
-- 	('Ремонт труб', ST_SetSRID(ST_MakePoint(41.463077, 52.718319), 4326), 2, 1, 1, 0, 0);
-- END;