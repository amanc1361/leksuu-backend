-- 0052 — dictionary lookup and review meanings (QA N09, N03).
-- 1) leksuu_norm_lemma: the SQL twin of domain.NormalizeLemma — lowercase,
--    trimmed, surrounding sentence punctuation removed, inner spaces collapsed.
--    "Hello!", "Hi!" and "Meet" resolve to "hello", "hi" and "meet".
CREATE OR REPLACE FUNCTION leksuu_norm_lemma(t text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT regexp_replace(
           regexp_replace(
             regexp_replace(lower(btrim(coalesce(t, ''))), '^["“¿¡[:space:]]+', ''),
             '[.!?,;:؟،"”…[:space:]]+$', ''),
           '[[:space:]]+', ' ', 'g')
$$;
CREATE INDEX IF NOT EXISTS idx_vocab_norm_lemma ON vocabulary_items (language, leksuu_norm_lemma(lemma));

-- 2) Core A1 entries the QA lookups could not find. Meanings are first drafts
--    for editorial review (fa = Persian, ckb = Central Kurdish).
INSERT INTO vocabulary_items (language, item_kind, lemma, pronunciation, meaning, example) VALUES
  ('en', 'word', 'hello', '/həˈloʊ/', '{"fa": "سلام", "ckb": "سڵاو"}', 'Hello, how are you?'),
  ('en', 'word', 'hi',    '/haɪ/',    '{"fa": "سلام (خودمانی)", "ckb": "سڵاو (ناڕەسمی)"}', 'Hi! I''m Sara.'),
  ('en', 'word', 'meet',  '/miːt/',   '{"fa": "ملاقات کردن؛ آشنا شدن", "ckb": "بینین؛ ناسین"}', 'Nice to meet you.'),
  ('en', 'word', 'are',   '/ɑːr/',    '{"fa": "هستی / هستید / هستند (فعل be برای you، we، they)", "ckb": "ـیت / ـن (کرداری be بۆ you، we، they)"}', 'Where are you from?')
ON CONFLICT (language, item_kind, lemma) DO UPDATE
  SET meaning = EXCLUDED.meaning || vocabulary_items.meaning
  WHERE vocabulary_items.meaning = '{}'::jsonb
     OR NOT (vocabulary_items.meaning ? 'fa');

-- 3) Phrase/chunk cards saved from lessons with no native meaning ("Hello!",
--    "Hi!") borrow the matching word's meaning, so revealing them shows an answer.
UPDATE vocabulary_items p
   SET meaning = w.meaning || p.meaning
  FROM vocabulary_items w
 WHERE w.language = p.language
   AND w.item_kind = 'word'
   AND p.item_kind <> 'word'
   AND w.meaning ? 'fa'
   AND NOT (p.meaning ? 'fa')
   AND leksuu_norm_lemma(p.lemma) = leksuu_norm_lemma(w.lemma);
