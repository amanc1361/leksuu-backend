-- 0056 — group classes are sold per seat at the teacher's own price, like 1:1
-- sessions (0055): paid from class credit or at the gateway (a seat is held
-- during payment), refunded as credit when leaving more than 24h ahead, and
-- the teacher earns their share per paid seat. Classes without a price stay
-- open to learners with an old class subscription only.

CREATE TABLE IF NOT EXISTS class_prices (
  class_id     text PRIMARY KEY REFERENCES class_sessions(id) ON DELETE CASCADE,
  price_toman  int  NOT NULL CHECK (price_toman > 0)
);

-- Every paid seat, kept even after the learner leaves (for refunds/earnings).
CREATE TABLE IF NOT EXISTS class_enrollment_prices (
  class_id        text NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
  student_id      text NOT NULL,
  price_toman     int  NOT NULL CHECK (price_toman >= 0),
  share_percent   int  NOT NULL CHECK (share_percent BETWEEN 0 AND 100),
  paid_via        text NOT NULL CHECK (paid_via IN ('credit', 'payment')),
  refunded_toman  int  NOT NULL DEFAULT 0 CHECK (refunded_toman >= 0),
  created_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (class_id, student_id)
);

-- A seat being paid at the gateway; it counts against capacity while pending.
CREATE TABLE IF NOT EXISTS class_checkouts (
  id           text PRIMARY KEY,
  class_id     text NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
  student_id   text NOT NULL,
  price_toman  int  NOT NULL,
  status       text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'failed', 'expired')),
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_class_checkouts_class ON class_checkouts (class_id, status, created_at);
