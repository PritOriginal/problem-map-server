DELETE FROM types_marks 
WHERE name IN ('Освещение', 'Информационные и визуальные дефекты');

UPDATE types_marks
SET name = 'Инфраструктура'
WHERE type_mark_id = 2;

ALTER SEQUENCE types_marks_type_mark_id_seq RESTART WITH 4;