-- Stable identity for lessons originating from external JSON course bundles.
-- The partial unique index makes repeated imports safe without affecting
-- manually authored lessons, whose source_id remains NULL.
ALTER TABLE lessons ADD COLUMN source_id text;

CREATE UNIQUE INDEX lessons_unit_source_id_uq
    ON lessons(unit_id, source_id)
    WHERE source_id IS NOT NULL AND source_id <> '';
