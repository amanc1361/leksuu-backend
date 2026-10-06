-- 0049 — add the multi-blank "fill the gaps" exercise type. A sentence with
-- two or more separate gaps, each with its own ordered answer key, graded as a
-- complete set (every gap must be correct). Single-gap sentences keep using
-- fill_blank.
ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check CHECK (type IN
  ('mcq','fill_blank','match','reorder','listen_choose',
   'translate','speak','listen_answer',
   'true_false','dictation','sentence_builder','match_image',
   'select_words','fill_gaps'));
