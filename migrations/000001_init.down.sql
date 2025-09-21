
CREATE TABLE regions (
    region_id SERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL,
    geom GEOMETRY NOT NULL
);

CREATE TABLE cities (
    city_id SERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL,
    region_id INTEGER,
    geom GEOMETRY NOT NULL,
    CONSTRAINT fk_region FOREIGN KEY (region_id) REFERENCES regions(region_id)
);

CREATE TABLE districts (
    district_id SERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL,
    city_id INTEGER,
    geom GEOMETRY NOT NULL,
    CONSTRAINT fk_city FOREIGN KEY (city_id) REFERENCES cities(city_id)
);

CREATE TABLE types_marks (
    type_mark_id SERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL
);

CREATE TABLE users (
    user_id SERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL,
    rating INTEGER
);

CREATE TABLE marks (
    mark_id SERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL,
    geom GEOMETRY NOT NULL,
    type_mark_id INTEGER,
    user_id INTEGER,
    district_id INTEGER,
    number_votes INTEGER,
    number_checks INTEGER,
    CONSTRAINT fk_type_mark FOREIGN KEY (type_mark_id) REFERENCES types_marks(type_mark_id),
    CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES users(user_id),
    CONSTRAINT fk_district FOREIGN KEY (district_id) REFERENCES districts(district_id)
);

CREATE TABLE tasks (
    task_id SERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL,
    user_id INTEGER,
    CONSTRAINT fk_user_task FOREIGN KEY (task_id) REFERENCES users(user_id)
);