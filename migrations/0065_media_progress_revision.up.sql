ALTER TABLE media_learner_progress
    ADD COLUMN revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0);
