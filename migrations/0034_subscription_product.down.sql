ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_pkey;
ALTER TABLE subscriptions ADD PRIMARY KEY (user_id);
ALTER TABLE subscriptions DROP COLUMN IF EXISTS product;
ALTER TABLE payments DROP COLUMN IF EXISTS product;
