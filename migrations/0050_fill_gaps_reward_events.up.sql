-- 0050 — multi-gap exercises and one-time reward events.
-- 1) fill_gaps — a sentence with several independent gaps. Every gap has its
--    own accepted answers and all of them must be right (Busuu 26b). Before
--    this type existed such sentences were imported as a one-choice mcq that
--    kept only the first gap's answer; a re-import converts them in place.
ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_type_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_type_check CHECK (type IN
  ('mcq','fill_blank','match','reorder','listen_choose',
   'translate','speak','listen_answer',
   'true_false','dictation','sentence_builder','match_image',
   'select_words','fill_gaps'));

-- 2) reward_events — one row per rewarded event (e.g. "lesson:<id>"), so a
--    retried or repeated request can never grant the same reward twice.
CREATE TABLE IF NOT EXISTS reward_events (
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_key  text NOT NULL,
  xp         int  NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, event_key)
);

-- Lessons completed before this migration were already rewarded.
INSERT INTO reward_events (user_id, event_key, xp)
SELECT DISTINCT user_id, 'lesson:' || entity_id::text, 0
  FROM user_progress
 WHERE entity_type = 'lesson' AND status = 'completed'
ON CONFLICT DO NOTHING;
