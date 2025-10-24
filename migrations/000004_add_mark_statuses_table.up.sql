CREATE TABLE mark_statuses (
    mark_status_id SERIAL PRIMARY KEY,
    name VARCHAR(40) NOT NULL
);

INSERT INTO
	mark_statuses (name)
VALUES
	('Неподтверждённая'), ('Подтверждённая'), ('Выполненная');

ALTER TABLE marks ADD COLUMN mark_status_id INTEGER DEFAULT 1 NOT NULL;

ALTER TABLE marks ADD CONSTRAINT fk_mark_status FOREIGN KEY (mark_status_id) REFERENCES mark_statuses(mark_status_id);