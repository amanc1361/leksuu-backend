-- 0042 — Daily Mix: date the user last completed the guided daily session
-- (review + shadowing + media). Mirrors last_reward's once-per-day semantics.
ALTER TABLE user_gamification ADD COLUMN IF NOT EXISTS last_mix date;
