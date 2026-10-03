-- 0049 — stable source identities for re-imports, and localized content.
-- 1) courses.source_id / units.source_id: the external course pack and chapter
--    an import came from (e.g. a Busuu chapter id). Re-importing the same
--    archive finds them again instead of matching by (editable) title.
--    Hand-authored rows keep NULL, so they are never touched by an import.
-- 2) Lesson source ids are looked up course-wide: a lesson imported into the
--    wrong unit by an older importer is moved, not duplicated.
-- 3) content_translations holds translations of imported explanatory text
--    (grammar tips) into the learner's language, keyed by a hash of the exact
--    source string. It is separate from lesson_items.payload, so the original
--    source text is never overwritten and a re-import never loses translations.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS source_id text;
CREATE UNIQUE INDEX IF NOT EXISTS courses_source_id_uq
    ON courses(source_id) WHERE source_id IS NOT NULL AND source_id <> '';

ALTER TABLE units ADD COLUMN IF NOT EXISTS source_id text;
CREATE UNIQUE INDEX IF NOT EXISTS units_course_source_id_uq
    ON units(course_id, source_id) WHERE source_id IS NOT NULL AND source_id <> '';

CREATE INDEX IF NOT EXISTS lessons_course_source_id_idx
    ON lessons(course_id, source_id) WHERE source_id IS NOT NULL AND source_id <> '';

CREATE TABLE IF NOT EXISTS content_translations (
  source_hash  text NOT NULL,            -- sha256 hex of source_text
  locale       text NOT NULL,            -- learner locale: fa | ckb | en ...
  source_lang  text NOT NULL DEFAULT 'en',
  source_text  text NOT NULL,
  text         text NOT NULL,
  origin       text NOT NULL DEFAULT 'human' CHECK (origin IN ('human','machine','source')),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (source_hash, locale)
);
