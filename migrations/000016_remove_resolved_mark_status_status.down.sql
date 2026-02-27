UPDATE mark_statuses 
SET mark_status_id = mark_status_id + 100
WHERE mark_status_id IN (4, 5, 6, 7);

UPDATE mark_statuses
SET mark_status_id = CASE
    WHEN name = 'На проверке' THEN 4
    WHEN name = 'Переоткрытая' THEN 5
    WHEN name = 'Закрытая' THEN 6
    WHEN name = 'Опровергнутая' THEN 7
END
WHERE name IN ('На проверке', 'Переоткрытая', 'Закрытая', 'Опровергнутая');

UPDATE mark_statuses 
SET mark_status_id = mark_status_id - 100
WHERE mark_status_id > 100;

INSERT INTO mark_statuses (mark_status_id, name)
VALUES (3, 'Решённая')
ON CONFLICT (mark_status_id) DO NOTHING;