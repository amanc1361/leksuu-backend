-- Phase 4: a speaking (pronunciation) exercise attached to the Greetings lesson.
-- Reuses the existing Greetings exercise item (…0000b3). Idempotent.
INSERT INTO exercises (id, lesson_item_id, type, prompt, answer_key, difficulty) VALUES
  ('00000000-0000-0000-0000-000000250001', '00000000-0000-0000-0000-0000000000b3', 'speak',
   '{"text":"Hello! How are you?","hint":{"fa":"واضح و بلند و شمرده بگو","ckb":"بە ڕوونی و بەرز بیڵێ"}}'::jsonb,
   '{"min_score":60}'::jsonb, 1)
ON CONFLICT (id) DO NOTHING;
