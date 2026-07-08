-- Per-user, per-type notification delivery preferences. Only explicit overrides
-- are stored; an absent row means the type is enabled (the default).
CREATE TABLE IF NOT EXISTS notification_prefs (
  user_id text NOT NULL,
  type    text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  PRIMARY KEY (user_id, type)
);
