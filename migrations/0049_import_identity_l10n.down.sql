DROP TABLE IF EXISTS content_translations;
DROP INDEX IF EXISTS lessons_course_source_id_idx;
DROP INDEX IF EXISTS units_course_source_id_uq;
ALTER TABLE units DROP COLUMN IF EXISTS source_id;
DROP INDEX IF EXISTS courses_source_id_uq;
ALTER TABLE courses DROP COLUMN IF EXISTS source_id;
