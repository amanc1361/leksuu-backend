-- 0005 — Media learning (podcasts) + graded reader stories.
-- Both share the segment + click-to-translate + click-to-SRS pattern that wins
-- the Iranian market (Zabanshenas-style): click a word/sentence -> meaning,
-- pronunciation, example -> send to Leitner (srs_cards).

CREATE TABLE podcasts (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  language     text NOT NULL,
  cefr_level   text NOT NULL,
  title        text NOT NULL,
  description  text,
  difficulty   real,                          -- computed at ingest
  cover_key    text,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_podcasts_lang_level ON podcasts(language, cefr_level);

CREATE TABLE podcast_episodes (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  podcast_id   uuid NOT NULL REFERENCES podcasts(id) ON DELETE CASCADE,
  position     int  NOT NULL,
  title        text NOT NULL,
  audio_key    text NOT NULL,                 -- HLS master
  duration_ms  int  NOT NULL,
  summary      jsonb,                          -- AI-generated, localized
  status       text NOT NULL DEFAULT 'processing'
               CHECK (status IN ('processing','ready','failed')),
  UNIQUE (podcast_id, position)
);

-- Generic timed segment table used by BOTH podcasts and stories.
CREATE TABLE media_segments (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  parent_type  text NOT NULL CHECK (parent_type IN ('episode','story')),
  parent_id    uuid NOT NULL,
  idx          int  NOT NULL,
  start_ms     int,                            -- null for pure reading
  end_ms       int,
  text         text NOT NULL,
  translation  jsonb,                          -- lazy-filled & cached per locale
  UNIQUE (parent_type, parent_id, idx)
);
CREATE INDEX idx_segments_parent ON media_segments(parent_type, parent_id, idx);

CREATE TABLE transcripts (
  parent_type   text NOT NULL CHECK (parent_type IN ('episode','story')),
  parent_id     uuid NOT NULL,
  full_text     text NOT NULL,
  word_count    int  NOT NULL,
  vocab_extracted boolean NOT NULL DEFAULT false,
  PRIMARY KEY (parent_type, parent_id)
);

-- Graded reader stories (with optional narration audio).
CREATE TABLE stories (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  base_id      uuid REFERENCES stories_base(id) ON DELETE SET NULL,
  language     text NOT NULL,
  cefr_level   text NOT NULL,
  title        text NOT NULL,
  cover_key    text,
  audio_key    text,                           -- optional narration
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_stories_lang_level ON stories(language, cefr_level);
