-- 0006 — ELSA-style speaking & pronunciation.
-- Phoneme-level scoring + a per-user "problem sounds" profile that drives
-- targeted drills (the core of what makes ELSA effective).

CREATE TABLE speaking_sessions (
  id            uuid DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL,
  mode          text NOT NULL CHECK (mode IN
                 ('shadowing','scenario','free','mini_story_answer','read_aloud')),
  scenario      text,
  reference_text text,
  audio_key     text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE INDEX idx_speaking_user ON speaking_sessions(user_id, created_at DESC);

CREATE TABLE pronunciation_scores (
  session_id    uuid NOT NULL,
  created_at    timestamptz NOT NULL,
  accuracy      real, fluency real, completeness real, prosody real,
  word_scores   jsonb,                          -- [{word,score}]
  phoneme_scores jsonb,                         -- [{phoneme,score,offset_ms}]
  PRIMARY KEY (session_id, created_at)
) PARTITION BY RANGE (created_at);

-- IPA / ARPAbet inventory (reference).
CREATE TABLE phonemes (
  symbol   text PRIMARY KEY,                    -- e.g. 'θ','æ','r'
  ipa      text,
  example  text,
  mouth_hint jsonb                              -- ELSA-style "mouth shape" guidance
);

-- The learner's running per-phoneme accuracy = their "problem sounds" profile.
CREATE TABLE user_phoneme_profile (
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  phoneme     text NOT NULL REFERENCES phonemes(symbol),
  attempts    int  NOT NULL DEFAULT 0,
  avg_score   real NOT NULL DEFAULT 0,
  last_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, phoneme)
);

-- Targeted drills, surfaced when a phoneme's avg_score is low.
CREATE TABLE pronunciation_drills (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  phoneme     text NOT NULL REFERENCES phonemes(symbol),
  language    text NOT NULL,
  prompt_text text NOT NULL,
  audio_key   text,
  difficulty  smallint NOT NULL DEFAULT 1
);
CREATE INDEX idx_drills_phoneme ON pronunciation_drills(phoneme);
