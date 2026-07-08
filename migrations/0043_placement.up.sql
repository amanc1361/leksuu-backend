-- 0043 — placement: the user's CEFR level from the short placement test.
CREATE TABLE IF NOT EXISTS user_placement (
  user_id  uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  cefr     text NOT NULL CHECK (cefr IN ('A1','A2','B1','B2','C1')),
  taken_at timestamptz NOT NULL DEFAULT now()
);
