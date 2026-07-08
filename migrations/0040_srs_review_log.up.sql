-- Per-review activity log powering the daily review goal and the review streak.
CREATE TABLE IF NOT EXISTS srs_review_log (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  reviewed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_srs_review_log_user_time ON srs_review_log(user_id, reviewed_at);
