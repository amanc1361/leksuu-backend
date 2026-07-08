-- 0011 — DEFAULT partitions so inserts work out-of-the-box.
-- The worker creates monthly RANGE partitions ahead of time for performance;
-- the DEFAULT catches anything not yet covered.
CREATE TABLE user_progress_default        PARTITION OF user_progress        DEFAULT;
CREATE TABLE speaking_sessions_default     PARTITION OF speaking_sessions     DEFAULT;
CREATE TABLE pronunciation_scores_default  PARTITION OF pronunciation_scores  DEFAULT;
