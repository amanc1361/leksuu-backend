-- 0004 — AJ Hoge "Effortless English" engine.
-- Rule 5: Point-of-View stories (same story in different tenses).
-- Rule 7: Listen-and-Answer mini-stories (narrator asks, learner answers aloud).

-- A base narrative; its POV variants share this parent.
CREATE TABLE stories_base (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  language     text NOT NULL,
  cefr_level   text NOT NULL,
  title        text NOT NULL,
  synopsis     text,
  created_at   timestamptz NOT NULL DEFAULT now()
);

-- Rule 5: each variant retells the SAME story in a different grammatical frame,
-- so grammar is absorbed implicitly, never taught as rules.
CREATE TABLE pov_variants (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  base_id      uuid NOT NULL REFERENCES stories_base(id) ON DELETE CASCADE,
  pov          text NOT NULL CHECK (pov IN
                ('present','past','present_perfect','future','past_perfect')),
  body         text NOT NULL,
  audio_key    text,
  duration_ms  int,
  UNIQUE (base_id, pov)
);

-- Rule 7: a listen-and-answer mini-story = an audio + an ordered set of
-- simple questions the narrator asks; the learner answers aloud (short answers ok).
CREATE TABLE mini_stories (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  base_id      uuid REFERENCES stories_base(id) ON DELETE SET NULL,
  language     text NOT NULL,
  cefr_level   text NOT NULL,
  title        text NOT NULL,
  transcript   text NOT NULL,
  audio_key    text NOT NULL,
  duration_ms  int  NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE mini_story_questions (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  mini_story_id uuid NOT NULL REFERENCES mini_stories(id) ON DELETE CASCADE,
  idx           int  NOT NULL,                 -- order asked
  pause_ms      int  NOT NULL,                 -- where to pause the audio
  q_type        text NOT NULL CHECK (q_type IN
                 ('yes_no','who','what','when','where','why','how','tag','or')),
  question      text NOT NULL,
  expected      text NOT NULL,                 -- short canonical answer
  accepts       text[] NOT NULL DEFAULT '{}',  -- other accepted short answers
  UNIQUE (mini_story_id, idx)
);

-- Rule 4: deep, repeated listening. Track how many times a learner has
-- looped a given piece of content (drives "ready to advance" logic).
CREATE TABLE deep_listen_log (
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  content_type text NOT NULL,                  -- 'mini_story','pov_variant','podcast'
  content_id  uuid NOT NULL,
  reps        int  NOT NULL DEFAULT 0,
  last_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, content_type, content_id)
);
