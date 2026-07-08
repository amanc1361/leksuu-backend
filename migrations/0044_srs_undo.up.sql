-- One-level undo for reviews: the card's pre-review scheduling state.
ALTER TABLE srs_cards ADD COLUMN IF NOT EXISTS prev_state jsonb;
