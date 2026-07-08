-- Per-user SRS preferences. For now just the daily review goal (was a fixed 10).
CREATE TABLE IF NOT EXISTS srs_settings (
  user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  daily_goal int NOT NULL DEFAULT 10,
  updated_at timestamptz NOT NULL DEFAULT now()
);
