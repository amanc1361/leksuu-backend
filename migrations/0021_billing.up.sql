-- Monetization: subscriptions, payments, and free-tier daily usage.
CREATE TABLE subscriptions (
  user_id    uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  plan       text NOT NULL DEFAULT 'free',
  status     text NOT NULL DEFAULT 'expired',
  period_end timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- Drop any stale 'payments' table from the superseded 0008 migration
-- (only present on databases migrated before this fix; no-op otherwise).
DROP TABLE IF EXISTS payments CASCADE;
CREATE TABLE payments (
  authority    text PRIMARY KEY,
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  plan_id      text NOT NULL,
  amount_toman int  NOT NULL,
  status       text NOT NULL DEFAULT 'pending',
  ref_id       text,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE daily_usage (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  day     date NOT NULL,
  action  text NOT NULL,
  count   int  NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, day, action)
);
