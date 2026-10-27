ALTER TABLE lessons ADD COLUMN IF NOT EXISTS source_id text;

CREATE UNIQUE INDEX IF NOT EXISTS idx_lessons_unit_source_id
    ON lessons (unit_id, source_id)
    WHERE source_id IS NOT NULL;
