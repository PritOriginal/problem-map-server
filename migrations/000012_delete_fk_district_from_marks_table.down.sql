ALTER TABLE marks ADD COLUMN district_id INTEGER;
ALTER TABLE marks ADD CONSTRAINT fk_district FOREIGN KEY (district_id) REFERENCES districts(district_id);