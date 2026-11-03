-- Extend the durable media queue with target-language translation drafts.
ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_operation_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_operation_check
    CHECK (operation IN ('synchronize', 'translate'));
ALTER TABLE media_processing_jobs ADD COLUMN target_language text NOT NULL DEFAULT '';
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_target_language_check
    CHECK ((operation='synchronize' AND target_language='') OR
           (operation='translate' AND target_language ~ '^[a-z]{2,3}$'));
DROP INDEX media_processing_jobs_active_unique;
CREATE UNIQUE INDEX media_processing_jobs_active_unique
    ON media_processing_jobs(item_id,operation,target_language,payload_sha256)
    WHERE status IN ('pending','running','review');
