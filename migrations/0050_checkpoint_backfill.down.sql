-- Remove the rebuilt checkpoint steps (exercises cascade with their items).
DELETE FROM lesson_items WHERE source_ref = 'checkpoint-backfill';
