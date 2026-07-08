-- Allow 'grammar' as a vocabulary kind so a missed grammar exercise can enroll
-- its rule (e.g. "to be", "Present Simple") as a spaced-repetition review card.
ALTER TABLE vocabulary_items DROP CONSTRAINT IF EXISTS vocabulary_items_item_kind_check;
ALTER TABLE vocabulary_items ADD CONSTRAINT vocabulary_items_item_kind_check
  CHECK (item_kind IN ('word','phrase','chunk','collocation','grammar'));
