DROP INDEX IF EXISTS lesson_items_imported_idx;
DELETE FROM exercises WHERE type = 'select_words';
ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check CHECK (type IN
  ('mcq','fill_blank','match','reorder','listen_choose',
   'translate','speak','listen_answer',
   'true_false','dictation','sentence_builder','match_image'));

DELETE FROM lesson_items WHERE kind IN ('reading','writing','roleplay');
ALTER TABLE lesson_items DROP CONSTRAINT IF EXISTS lesson_items_kind_check;
ALTER TABLE lesson_items ADD CONSTRAINT lesson_items_kind_check CHECK (kind IN
  ('text','image','audio','video','exercise','speaking',
   'dialogue','mini_story','pov_story','story','podcast','grammar'));
