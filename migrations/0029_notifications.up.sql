-- In-app notifications: TYPE + structured DATA (localized client-side), per user.
CREATE TABLE IF NOT EXISTS notifications (
  id         text PRIMARY KEY,
  user_id    text NOT NULL,
  type       text NOT NULL,
  data       jsonb NOT NULL DEFAULT '{}'::jsonb,
  link       text NOT NULL DEFAULT '',
  read       boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications(user_id) WHERE NOT read;
