-- Tutor applications: a self-applied tutor starts "pending" and only becomes
-- bookable once an admin approves them. Existing tutors are grandfathered in as
-- approved.
ALTER TABLE tutors ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'approved';
CREATE INDEX IF NOT EXISTS idx_tutors_status ON tutors(status);
