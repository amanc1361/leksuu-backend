-- 0003 — vocabulary + SRS. AJ Hoge Rule 1: store PHRASES/CHUNKS, not just words.
CREATE TABLE vocabulary_items (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  language      text NOT NULL,
  item_kind     text NOT NULL DEFAULT 'word'
                CHECK (item_kind IN ('word','phrase','chunk','collocation')),
  lemma         text NOT NULL,
  pronunciation text,
  meaning       jsonb NOT NULL,            -- {"fa":"…","ckb":"…"} localized
  example       text,
  audio_key     text,
  UNIQUE (language, item_kind, lemma)
);
CREATE INDEX idx_vocab_lang ON vocabulary_items(language, item_kind);

-- One SRS card per (user, vocab). Smart Leitner / FSRS state. Hot table.
CREATE TABLE srs_cards (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  vocab_id      uuid NOT NULL REFERENCES vocabulary_items(id) ON DELETE CASCADE,
  box           smallint NOT NULL DEFAULT 1,   -- Leitner box 1..N
  ease          real NOT NULL DEFAULT 2.5,     -- FSRS/SM-2
  interval_days int  NOT NULL DEFAULT 0,
  reps          int  NOT NULL DEFAULT 0,
  lapses        int  NOT NULL DEFAULT 0,
  due_at        timestamptz NOT NULL DEFAULT now(),
  source        text,                          -- 'lesson','podcast','story','mini_story'
  source_ref    uuid,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, vocab_id)
);
CREATE INDEX idx_srs_due ON srs_cards(user_id, due_at);
