ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_operation_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_operation_check
    CHECK (operation IN ('synchronize','translate','transcode'));
ALTER TABLE media_processing_jobs DROP CONSTRAINT media_processing_jobs_target_language_check;
ALTER TABLE media_processing_jobs ADD CONSTRAINT media_processing_jobs_target_language_check
    CHECK ((operation IN ('synchronize','transcode') AND target_language='') OR
           (operation='translate' AND target_language ~ '^[a-z]{2,3}$'));

CREATE TABLE media_learner_progress (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_id uuid NOT NULL REFERENCES lesson_items(id) ON DELETE CASCADE,
    target_language text NOT NULL,
    position_seconds double precision NOT NULL DEFAULT 0 CHECK (position_seconds >= 0 AND position_seconds < 864000),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    correct_attempts integer NOT NULL DEFAULT 0 CHECK (correct_attempts >= 0 AND correct_attempts <= attempts),
    completed_cue_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    completed boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id,item_id,target_language)
);
CREATE INDEX media_learner_progress_item ON media_learner_progress(item_id);
