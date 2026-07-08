-- Context-anchored SRS: store the media moment a saved word came from so it can
-- be re-played in its original context at review time.
ALTER TABLE srs_cards ADD COLUMN clip jsonb;
