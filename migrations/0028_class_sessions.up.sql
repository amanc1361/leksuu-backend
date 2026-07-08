-- Group classes: one shared room, many enrolled students (live mesh today; an
-- SFU is the path to larger groups).
-- Drop any stale 'class_sessions' from the superseded 0008 migration
-- (only present on databases migrated before this fix; no-op otherwise).
DROP TABLE IF EXISTS class_sessions CASCADE;
CREATE TABLE IF NOT EXISTS class_sessions (
  id           text PRIMARY KEY,
  tutor_id     text NOT NULL,
  title        text NOT NULL,
  description  text NOT NULL DEFAULT '',
  start_utc    timestamptz NOT NULL,
  duration_min int  NOT NULL DEFAULT 30,
  capacity     int  NOT NULL DEFAULT 6,
  room_id      text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_class_sessions_tutor ON class_sessions(tutor_id, start_utc);
CREATE INDEX IF NOT EXISTS idx_class_sessions_start ON class_sessions(start_utc);

CREATE TABLE IF NOT EXISTS class_enrollments (
  class_id   text NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
  student_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (class_id, student_id)
);
CREATE INDEX IF NOT EXISTS idx_class_enrollments_student ON class_enrollments(student_id);
