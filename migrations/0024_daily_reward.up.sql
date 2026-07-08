-- Claimable daily reward (Duolingo-style chest): track the last day a user claimed.
ALTER TABLE user_gamification ADD COLUMN IF NOT EXISTS last_reward date;
