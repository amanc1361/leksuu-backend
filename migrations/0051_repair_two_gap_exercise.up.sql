-- 0051 — repair the already-imported two-blank exercise (QA N01).
-- Lesson 447cc5af… ("The singular forms of the verb 'to be'"), step 8:
--   "I____ from India. And where ____ you from?"
-- The old importer collapsed both blanks into one mcq keyed only on "'m", so a
-- single choice was graded correct and the second blank ("are", as the
-- exercise's own explanation states) was dropped. Convert it to fill_gaps with
-- one ordered key per blank and an option set that can answer both. Matched by
-- lesson id AND sentence, so it is a no-op on any other database. New imports
-- produce fill_gaps directly; other multi-blank records are repaired by
-- re-importing their lessons with the fixed importer.
UPDATE exercises e
   SET type = 'fill_gaps',
       prompt = (e.prompt - 'question' - 'options' - 'option_images')
                || jsonb_build_object(
                     'text', coalesce(e.prompt->>'question', e.prompt->>'text'),
                     'gap_options', jsonb_build_array(
                        CASE WHEN coalesce(e.prompt->'options', '[]'::jsonb) ? 'are'
                             THEN e.prompt->'options'
                             ELSE coalesce(e.prompt->'options', '["''m","''s","be"]'::jsonb) || '["are"]'::jsonb END,
                        CASE WHEN coalesce(e.prompt->'options', '[]'::jsonb) ? 'are'
                             THEN e.prompt->'options'
                             ELSE coalesce(e.prompt->'options', '["''m","''s","be"]'::jsonb) || '["are"]'::jsonb END)),
       answer_key = jsonb_build_object('gaps', jsonb_build_array(
                      jsonb_build_array('''m'),
                      jsonb_build_array('are')))
  FROM lesson_items li
 WHERE e.lesson_item_id = li.id
   AND li.lesson_id = '447cc5af-af0e-43aa-abbb-6b646e55cb78'
   AND e.type IN ('mcq', 'fill_blank')
   AND coalesce(e.prompt->>'question', e.prompt->>'text', '') ILIKE 'I%from India%where%you from%'
   AND array_length(regexp_split_to_array(coalesce(e.prompt->>'question', e.prompt->>'text', ''), '_+'), 1) = 3;
