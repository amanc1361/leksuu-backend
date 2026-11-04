-- Refuse rollback while completed automatic jobs still use the new state.
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM media_processing_jobs WHERE status='ready') THEN
    RAISE EXCEPTION 'Resolve ready media jobs before reverting migration 0060';
  END IF;
END $$;
DROP INDEX media_processing_jobs_active_unique;
CREATE UNIQUE INDEX media_processing_jobs_active_unique
    ON media_processing_jobs(item_id,operation,target_language,payload_sha256)
    WHERE status IN ('pending','running','review');
ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_status_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_status_check
    CHECK (status IN ('pending','running','review','failed','cancelled','approved'));
