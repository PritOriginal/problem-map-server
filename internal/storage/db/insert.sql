INSERT INTO districts (name, geom)
	SELECT name, ST_Transform(way, 4326) 
	FROM planet_osm_polygon;


INSERT INTO
	types_marks (name)
VALUES
	('Мусор'), ('Инфраструктура');

INSERT INTO 
	marks (name, geom, type_mark_id, user_id, district_id, number_votes, number_checks) 
VALUES
	('Свалка', ST_SetSRID(ST_MakePoint(41.402893, 52.700111), 4326), 1, 1, 2, 0. 0),
	('Ремонт труб', ST_SetSRID(ST_MakePoint(41.463077, 52.718319), 4326), 2, 1, 1, 0, 0);