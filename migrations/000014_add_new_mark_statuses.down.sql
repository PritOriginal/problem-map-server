DELETE FROM
    mark_statuses
WHERE
    name IN (
        'На проверке',
        'Переоткрытая',
        'Закрытая',
        'Опровергнутая'
    );

UPDATE
    mark_statuses
SET
    name = 'Выполненная'
WHERE
    mark_status_id = 3;