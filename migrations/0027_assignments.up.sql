-- Tutor homework: one row = one task for one student (+ its submission + grade).
CREATE TABLE IF NOT EXISTS assignments (
  id              text PRIMARY KEY,
  tutor_id        text NOT NULL,           -- tutor's user account id
  student_id      text NOT NULL,
  type            text NOT NULL,           -- task | lesson
  title           text NOT NULL,
  prompt          text NOT NULL DEFAULT '',
  lesson_id       text NOT NULL DEFAULT '',
  due_utc         timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  submission_text text NOT NULL DEFAULT '',
  submitted_at    timestamptz,
  score           int  NOT NULL DEFAULT -1, -- -1 = ungraded
  feedback        text NOT NULL DEFAULT '',
  graded_at       timestamptz
);
CREATE INDEX IF NOT EXISTS idx_assignments_tutor   ON assignments(tutor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_assignments_student ON assignments(student_id, created_at DESC);
