-- Additive, private workspace state. No changes to English content or grading.
BEGIN;
CREATE TABLE IF NOT EXISTS localization_admin.memory_entries (
 segment_id uuid PRIMARY KEY REFERENCES localization_admin.segments(id) ON DELETE CASCADE,
 signature text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS admin_memory_signature ON localization_admin.memory_entries(signature);
ALTER TABLE localization_admin.segments ADD COLUMN IF NOT EXISTS memory_source_id uuid REFERENCES localization_admin.segments(id) ON DELETE SET NULL;
ALTER TABLE localization_admin.tasks ADD COLUMN IF NOT EXISTS memory_key text;
ALTER TABLE localization_admin.tasks ADD COLUMN IF NOT EXISTS depends_on uuid REFERENCES localization_admin.tasks(id);
CREATE INDEX IF NOT EXISTS admin_memory_active ON localization_admin.tasks(memory_key) WHERE status IN ('pending','running','waiting');
CREATE TABLE IF NOT EXISTS localization_admin.publication_batches (
 id uuid PRIMARY KEY, actor text NOT NULL, locale text NOT NULL,
 snapshot jsonb NOT NULL, receipt jsonb,
 created_at timestamptz NOT NULL DEFAULT now()
);
COMMIT;
