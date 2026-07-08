-- Subscriptions become per-product (a separate "classes" subscription alongside
-- the existing course/premium one). Existing rows are the course subscription.
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS product text NOT NULL DEFAULT 'courses';
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_pkey;
ALTER TABLE subscriptions ADD PRIMARY KEY (user_id, product);

ALTER TABLE payments ADD COLUMN IF NOT EXISTS product text NOT NULL DEFAULT 'courses';
