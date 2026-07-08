-- 0015 — add the match-word-to-image exercise type (vocab, Mondly-style).
ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check CHECK (type IN
  ('mcq','fill_blank','match','reorder','listen_choose',
   'translate','speak','listen_answer',
   'true_false','dictation','sentence_builder','match_image'));
