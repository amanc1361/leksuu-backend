ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
DELETE FROM exercises WHERE type = 'match_image';
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check CHECK (type IN
  ('mcq','fill_blank','match','reorder','listen_choose',
   'translate','speak','listen_answer',
   'true_false','dictation','sentence_builder'));
