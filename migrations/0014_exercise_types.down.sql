-- Rollback: the new types can't exist under the old constraint, so drop those
-- exercises first, then restore the original CHECK.
ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
DELETE FROM exercises WHERE type IN ('true_false','dictation','sentence_builder');
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check CHECK (type IN
  ('mcq','fill_blank','match','reorder','listen_choose',
   'translate','speak','listen_answer'));
