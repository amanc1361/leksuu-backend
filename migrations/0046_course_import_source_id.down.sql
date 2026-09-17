DROP INDEX IF EXISTS lessons_unit_source_id_uq;
ALTER TABLE lessons DROP COLUMN IF EXISTS source_id;
