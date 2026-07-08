-- Dynamic subscription plans: admins can add/edit/remove course & class plans
-- from the console. Billing reads active plans live (replacing the old fixed
-- monthly/yearly config). (product, id) is the key so each product keeps simple
-- period ids ("monthly", "yearly", or any custom id like "quarterly").
CREATE TABLE subscription_plans (
  product      text NOT NULL CHECK (product IN ('courses','classes')),
  id           text NOT NULL,
  title        text NOT NULL,
  amount_toman int  NOT NULL CHECK (amount_toman >= 0),
  days         int  NOT NULL CHECK (days > 0),
  position     int  NOT NULL DEFAULT 0,
  active       boolean NOT NULL DEFAULT true,
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (product, id)
);

-- Seed the four plans that were previously hardcoded, so behavior is unchanged.
INSERT INTO subscription_plans (product, id, title, amount_toman, days, position) VALUES
  ('courses', 'monthly', 'ماهانه', 99000,  30,  1),
  ('courses', 'yearly',  'سالانه', 890000, 365, 2),
  ('classes', 'monthly', 'ماهانه', 149000, 30,  1),
  ('classes', 'yearly',  'سالانه', 1490000,365, 2)
ON CONFLICT (product, id) DO NOTHING;
