DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM media_learner_progress LIMIT 1) THEN
    RAISE EXCEPTION 'preserve or export media learner progress before rolling back 0062';
  END IF;
  IF EXISTS (SELECT 1 FROM media_processing_jobs WHERE operation='transcode' LIMIT 1) THEN
    RAISE EXCEPTION 'preserve transcode job history before rolling back 0062';
  END IF;
END $$;
DROP TABLE IF EXISTS media_learner_progress;
ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_operation_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_operation_check
    CHECK (operation IN ('synchronize','translate'));
ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_target_language_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_target_language_check
    CHECK ((operation='synchronize' AND target_language='') OR
           (operation='translate' AND target_language ~ '^[a-z]{2,3}$'));
