-- Worker completion and editorial approval are distinct states.
ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_status_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_status_check
    CHECK (status IN ('pending','running','review','ready','failed','cancelled','approved'));
DROP INDEX media_processing_jobs_active_unique;
CREATE UNIQUE INDEX media_processing_jobs_active_unique
    ON media_processing_jobs(item_id,operation,target_language,payload_sha256)
    WHERE status IN ('pending','running','review','ready');
