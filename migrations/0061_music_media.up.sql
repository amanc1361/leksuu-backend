ALTER TABLE lesson_items DROP CONSTRAINT IF EXISTS lesson_items_kind_check;
ALTER TABLE lesson_items ADD CONSTRAINT lesson_items_kind_check CHECK (kind IN
  ('text','image','audio','video','exercise','speaking','dialogue','mini_story',
   'pov_story','story','podcast','music','grammar','reading','writing','roleplay'));

ALTER TABLE media_processing_jobs DROP CONSTRAINT IF EXISTS media_processing_jobs_kind_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_kind_check
  CHECK (kind IN ('story','podcast','video','music'));
