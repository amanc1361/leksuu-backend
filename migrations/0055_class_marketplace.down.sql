DROP TABLE IF EXISTS session_checkouts;
DROP TABLE IF EXISTS credit_ledger;
DROP TABLE IF EXISTS learner_credits;
DROP TABLE IF EXISTS booking_prices;
ALTER TABLE tutors DROP CONSTRAINT IF EXISTS tutors_session_price_check;
ALTER TABLE tutors DROP COLUMN IF EXISTS session_price_toman;
