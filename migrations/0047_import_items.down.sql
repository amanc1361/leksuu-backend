ALTER TABLE lesson_items DROP COLUMN IF EXISTS source_ref;
DELETE FROM lesson_items WHERE kind = 'grammar';
ALTER TABLE lesson_items DROP CONSTRAINT IF EXISTS lesson_items_kind_check;
ALTER TABLE lesson_items ADD CONSTRAINT lesson_items_kind_check CHECK (kind IN
  ('text','image','audio','video','exercise','speaking',
   'dialogue','mini_story','pov_story','story','podcast'));
