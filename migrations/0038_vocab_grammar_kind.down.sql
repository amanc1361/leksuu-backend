-- Revert to the original kind set. Any 'grammar' rows must be reclassified first.
ALTER TABLE vocabulary_items DROP CONSTRAINT IF EXISTS vocabulary_items_item_kind_check;
ALTER TABLE vocabulary_items ADD CONSTRAINT vocabulary_items_item_kind_check
  CHECK (item_kind IN ('word','phrase','chunk','collocation'));
