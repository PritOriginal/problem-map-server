DELETE FROM
    mark_statuses
WHERE
    name = 'Решённая';

UPDATE
    mark_statuses
SET
    mark_status_id = CASE
        name
        WHEN 'На проверке' THEN 3
        WHEN 'Переоткрытая' THEN 4
        WHEN 'Закрытая' THEN 5
        WHEN 'Опровергнутая' THEN 6
    END
WHERE
    name IN (
        'На проверке',
        'Переоткрытая',
        'Закрытая',
        'Опровергнутая'
    );
