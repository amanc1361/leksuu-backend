-- Pronunciation scoring: per-user best/last/attempts per media-item transcript line.
CREATE TABLE pronunciation_line_scores (
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item_id    uuid NOT NULL REFERENCES lesson_items(id) ON DELETE CASCADE,
  line       int  NOT NULL,
  best_score int  NOT NULL DEFAULT 0,
  last_score int  NOT NULL DEFAULT 0,
  attempts   int  NOT NULL DEFAULT 0,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, item_id, line)
);
