-- 0047 — safer course imports.
-- 1) 'grammar' items (Busuu tip tables, grammar rules) were used by the app but
--    never allowed by the lesson_items CHECK, so inserting one failed on Postgres.
-- 2) source_ref marks items created by an import. A re-import replaces only
--    those rows and leaves items an admin authored by hand untouched.
ALTER TABLE lesson_items DROP CONSTRAINT IF EXISTS lesson_items_kind_check;
ALTER TABLE lesson_items ADD CONSTRAINT lesson_items_kind_check CHECK (kind IN
  ('text','image','audio','video','exercise','speaking',
   'dialogue','mini_story','pov_story','story','podcast','grammar'));

ALTER TABLE lesson_items ADD COLUMN IF NOT EXISTS source_ref text;

-- Items already inside imported lessons came from an import: mark them so the
-- next re-import replaces them instead of duplicating them.
UPDATE lesson_items li
   SET source_ref = 'legacy-import'
  FROM lessons l
 WHERE li.lesson_id = l.id
   AND l.source_id IS NOT NULL AND l.source_id <> ''
   AND li.source_ref IS NULL;
