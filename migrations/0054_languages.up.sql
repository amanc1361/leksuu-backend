-- 0054 — target languages become data the admin manages (add, enable/disable,
-- order) instead of a list in the source. Learners see enabled languages only;
-- a disabled language's courses are hidden from learners but kept for admins,
-- so content can be prepared before a language goes live.
CREATE TABLE IF NOT EXISTS languages (
  code       text PRIMARY KEY CHECK (code ~ '^[a-z]{2,3}$'),
  enabled    boolean NOT NULL DEFAULT true,
  position   integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO languages (code, enabled, position) VALUES
  ('en', true, 1), ('de', true, 2), ('fr', true, 3), ('tr', true, 4)
ON CONFLICT (code) DO NOTHING;

-- Any language already used by a course stays manageable.
INSERT INTO languages (code, enabled, position)
SELECT DISTINCT c.language, true, 100
  FROM courses c
 WHERE c.language ~ '^[a-z]{2,3}$'
ON CONFLICT (code) DO NOTHING;
