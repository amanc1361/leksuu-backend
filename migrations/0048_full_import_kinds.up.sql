-- 0048 — item and exercise kinds needed for a complete course import.
-- 1) reading  — a longer text (blog post, calendar, notice) to read before
--               its comprehension questions.
--    writing  — an open writing task, checked by the AI writing coach.
--    roleplay — a scenario-driven conversation with the AI tutor.
-- 2) select_words — pick every correct word from a set of tiles.
ALTER TABLE lesson_items DROP CONSTRAINT IF EXISTS lesson_items_kind_check;
ALTER TABLE lesson_items ADD CONSTRAINT lesson_items_kind_check CHECK (kind IN
  ('text','image','audio','video','exercise','speaking',
   'dialogue','mini_story','pov_story','story','podcast','grammar',
   'reading','writing','roleplay'));

ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check CHECK (type IN
  ('mcq','fill_blank','match','reorder','listen_choose',
   'translate','speak','listen_answer',
   'true_false','dictation','sentence_builder','match_image',
   'select_words'));

-- A re-import replaces every imported item of a lesson in one statement.
CREATE INDEX IF NOT EXISTS lesson_items_imported_idx
  ON lesson_items (lesson_id) WHERE source_ref IS NOT NULL;
