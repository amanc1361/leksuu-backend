-- Link a tutor profile to a user account so the tutor can log in and join their
-- own class rooms (and, later, manage students). Empty string = not yet linked.
ALTER TABLE tutors ADD COLUMN IF NOT EXISTS user_id text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_tutors_user ON tutors(user_id) WHERE user_id <> '';
