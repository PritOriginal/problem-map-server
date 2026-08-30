-- Machine-readable codes for the dictionaries and a translations table.
-- Codes are assigned by the current (Russian) name because ids differ
-- between databases (e.g. a locally added type shifts the sequence);
-- unknown rows get a synthetic code so the NOT NULL/UNIQUE constraints hold,
-- and duplicated names (hence duplicated codes) get an `_<id>` suffix from
-- the second row on, so the UNIQUE constraint cannot fail.

ALTER TABLE types_marks ADD COLUMN code TEXT;
UPDATE types_marks SET code = CASE name
    WHEN 'Мусор'                               THEN 'garbage'
    WHEN 'Инфраструктура'                      THEN 'infrastructure'
    WHEN 'Зелёные зоны и парки'                THEN 'green_zones'
    WHEN 'Дорога'                              THEN 'roads'
    WHEN 'Дороги'                              THEN 'roads'
    WHEN 'Освещение'                           THEN 'lighting'
    WHEN 'Информационные и визуальные дефекты' THEN 'visual_defects'
    ELSE 'type_' || type_mark_id
END;
UPDATE types_marks t SET code = t.code || '_' || t.type_mark_id
FROM (SELECT type_mark_id, row_number() OVER (PARTITION BY code ORDER BY type_mark_id) AS rn FROM types_marks) d
WHERE d.type_mark_id = t.type_mark_id AND d.rn > 1;
ALTER TABLE types_marks ALTER COLUMN code SET NOT NULL;
ALTER TABLE types_marks ADD CONSTRAINT types_marks_code_key UNIQUE (code);

ALTER TABLE mark_statuses ADD COLUMN code TEXT;
UPDATE mark_statuses SET code = CASE name
    WHEN 'Неподтверждённая' THEN 'unconfirmed'
    WHEN 'Подтверждённая'   THEN 'confirmed'
    WHEN 'На проверке'      THEN 'under_review'
    WHEN 'Переоткрытая'     THEN 'rediscovered'
    WHEN 'Закрытая'         THEN 'closed'
    WHEN 'Опровергнутая'    THEN 'refuted'
    WHEN 'В работе'         THEN 'in_progress'
    ELSE 'status_' || mark_status_id
END;
UPDATE mark_statuses t SET code = t.code || '_' || t.mark_status_id
FROM (SELECT mark_status_id, row_number() OVER (PARTITION BY code ORDER BY mark_status_id) AS rn FROM mark_statuses) d
WHERE d.mark_status_id = t.mark_status_id AND d.rn > 1;
ALTER TABLE mark_statuses ALTER COLUMN code SET NOT NULL;
ALTER TABLE mark_statuses ADD CONSTRAINT mark_statuses_code_key UNIQUE (code);

ALTER TABLE task_statuses ADD COLUMN code TEXT;
UPDATE task_statuses SET code = CASE name
    WHEN 'Выдано'     THEN 'issued'
    WHEN 'Выполнено'  THEN 'completed'
    WHEN 'Просрочено' THEN 'overdue'
    ELSE 'status_' || status_id
END;
UPDATE task_statuses t SET code = t.code || '_' || t.status_id
FROM (SELECT status_id, row_number() OVER (PARTITION BY code ORDER BY status_id) AS rn FROM task_statuses) d
WHERE d.status_id = t.status_id AND d.rn > 1;
ALTER TABLE task_statuses ALTER COLUMN code SET NOT NULL;
ALTER TABLE task_statuses ADD CONSTRAINT task_statuses_code_key UNIQUE (code);

CREATE TABLE translations (
    entity    TEXT    NOT NULL,
    entity_id INTEGER NOT NULL,
    lang      CHAR(2) NOT NULL,
    name      TEXT    NOT NULL,
    PRIMARY KEY (entity, entity_id, lang)
);

-- Russian: the current names.
INSERT INTO translations (entity, entity_id, lang, name)
SELECT 'mark_type', type_mark_id, 'ru', name FROM types_marks;
INSERT INTO translations (entity, entity_id, lang, name)
SELECT 'mark_status', mark_status_id, 'ru', name FROM mark_statuses;
INSERT INTO translations (entity, entity_id, lang, name)
SELECT 'task_status', status_id, 'ru', name FROM task_statuses;

-- English, by code. Rows without a known code have no English translation
-- and fall back to Russian at read time.
INSERT INTO translations (entity, entity_id, lang, name)
SELECT 'mark_type', type_mark_id, 'en', t.en
FROM types_marks
JOIN (VALUES
    ('garbage',        'Garbage'),
    ('infrastructure', 'Infrastructure'),
    ('green_zones',    'Green zones and parks'),
    ('roads',          'Roads'),
    ('lighting',       'Lighting'),
    ('visual_defects', 'Information and visual defects')
) AS t(code, en) ON t.code = types_marks.code;

INSERT INTO translations (entity, entity_id, lang, name)
SELECT 'mark_status', mark_status_id, 'en', t.en
FROM mark_statuses
JOIN (VALUES
    ('unconfirmed',  'Unconfirmed'),
    ('confirmed',    'Confirmed'),
    ('under_review', 'Under review'),
    ('rediscovered', 'Rediscovered'),
    ('closed',       'Closed'),
    ('refuted',      'Refuted'),
    ('in_progress',  'In progress')
) AS t(code, en) ON t.code = mark_statuses.code;

INSERT INTO translations (entity, entity_id, lang, name)
SELECT 'task_status', status_id, 'en', t.en
FROM task_statuses
JOIN (VALUES
    ('issued',    'Issued'),
    ('completed', 'Completed'),
    ('overdue',   'Overdue')
) AS t(code, en) ON t.code = task_statuses.code;
