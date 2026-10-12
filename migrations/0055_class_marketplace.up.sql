-- 0055 — online classes are sold per session at the teacher's own price.
-- The platform keeps a share set in admin settings; the teacher's part is
-- recorded per booking. Cancelling more than 24h ahead refunds the learner as
-- credit (usable for later sessions); later cancellations are not refunded.
-- Bookings made under the old class subscription keep working (paid_via =
-- 'subscription') and are paid out at the old hourly rate.

ALTER TABLE tutors ADD COLUMN IF NOT EXISTS session_price_toman int NOT NULL DEFAULT 0;
ALTER TABLE tutors DROP CONSTRAINT IF EXISTS tutors_session_price_check;
ALTER TABLE tutors ADD CONSTRAINT tutors_session_price_check CHECK (session_price_toman >= 0);

-- What each booking cost and how the money splits (snapshotted at booking).
CREATE TABLE IF NOT EXISTS booking_prices (
  booking_id      text PRIMARY KEY,
  price_toman     int  NOT NULL CHECK (price_toman >= 0),
  share_percent   int  NOT NULL CHECK (share_percent BETWEEN 0 AND 100),
  paid_via        text NOT NULL CHECK (paid_via IN ('credit', 'payment')),
  refunded_toman  int  NOT NULL DEFAULT 0 CHECK (refunded_toman >= 0),
  created_at      timestamptz NOT NULL DEFAULT now()
);

-- A learner's class credit (refunds), with an audit trail of every change.
CREATE TABLE IF NOT EXISTS learner_credits (
  user_id        text PRIMARY KEY,
  balance_toman  int  NOT NULL DEFAULT 0 CHECK (balance_toman >= 0),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS credit_ledger (
  id           bigserial PRIMARY KEY,
  user_id      text NOT NULL,
  delta_toman  int  NOT NULL,
  reason       text NOT NULL,
  booking_id   text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_credit_ledger_user ON credit_ledger (user_id, created_at DESC);

-- A session being paid at the gateway: the slot is held until it resolves.
CREATE TABLE IF NOT EXISTS session_checkouts (
  id           text PRIMARY KEY,
  learner_id   text NOT NULL,
  slot_id      text NOT NULL,
  price_toman  int  NOT NULL,
  status       text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'failed', 'expired')),
  booking_id   text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_session_checkouts_slot ON session_checkouts (slot_id, status);
