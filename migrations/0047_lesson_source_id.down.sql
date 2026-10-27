DROP INDEX IF EXISTS idx_lessons_unit_source_id;

ALTER TABLE lessons DROP COLUMN IF EXISTS source_id;
