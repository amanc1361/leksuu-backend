-- Freemium lesson access: which lessons a user has unlocked.
-- source 'free' counts only for its day; 'gem' is a permanent paid unlock.
CREATE TABLE lesson_unlocks (
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  lesson_id  uuid NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
  source     text NOT NULL,        -- 'free' | 'gem'
  day        date NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, lesson_id)
);
CREATE INDEX idx_lesson_unlocks_free ON lesson_unlocks (user_id, day) WHERE source = 'free';
