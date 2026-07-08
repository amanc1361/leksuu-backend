-- 0014 — broaden the interactive exercise palette (Busuu/Mondly/ELSA-inspired):
-- true/false, dictation (type what you hear), sentence builder (word tiles).
-- listen_choose already existed in 0002.
ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check CHECK (type IN
  ('mcq','fill_blank','match','reorder','listen_choose',
   'translate','speak','listen_answer',
   'true_false','dictation','sentence_builder'));
