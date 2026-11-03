DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM media_processing_jobs WHERE operation='translate') THEN
        RAISE EXCEPTION 'archive translation job history before rolling back migration 0058';
    END IF;
END $$;
DROP INDEX media_processing_jobs_active_unique;
CREATE UNIQUE INDEX media_processing_jobs_active_unique
    ON media_processing_jobs(item_id,operation,payload_sha256)
    WHERE status IN ('pending','running','review');
ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_target_language_check;
ALTER TABLE media_processing_jobs DROP COLUMN target_language;
ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_operation_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_operation_check
    CHECK (operation IN ('synchronize'));
