-- Smart leech handling: a card that crosses the leech threshold is auto-suspended
-- (removed from the due queue) until the learner explicitly relearns it.
ALTER TABLE srs_cards ADD COLUMN IF NOT EXISTS suspended boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_srs_due_active ON srs_cards(user_id, due_at) WHERE suspended = false;
