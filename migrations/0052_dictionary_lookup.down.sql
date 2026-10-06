DROP INDEX IF EXISTS idx_vocab_norm_lemma;
DROP FUNCTION IF EXISTS leksuu_norm_lemma(text);
-- Backfilled meanings are content; they are intentionally left in place.
