DROP INDEX IF EXISTS idx_srs_due_active;
ALTER TABLE srs_cards DROP COLUMN IF EXISTS suspended;
