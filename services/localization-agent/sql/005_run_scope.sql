-- Additive upgrade: preserve selected lessons even when all fields are skipped.
BEGIN;
ALTER TABLE localization_admin.runs ADD COLUMN IF NOT EXISTS scope jsonb NOT NULL DEFAULT '[]';
CREATE INDEX IF NOT EXISTS admin_tasks_run ON localization_admin.tasks(run_id);
COMMIT;
