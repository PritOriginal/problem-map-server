INSERT INTO
    mark_statuses (name)
VALUES
    ('На проверке'),
    ('Переоткрытая'),
    ('Закрытая'),
    ('Опровергнутая');

UPDATE
    mark_statuses
SET
    name = 'Решённая'
WHERE
    mark_status_id = 3;