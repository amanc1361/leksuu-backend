-- Demo content seed (idempotent). Gives a fresh database a starter course so the
-- app is not empty on first run. Safe to re-run: fixed UUIDs + ON CONFLICT.
-- Admin users are provisioned via ADMIN_EMAILS at registration, not here.
BEGIN;

INSERT INTO courses (id, language, cefr_level, title, description, version, published) VALUES
  ('00000000-0000-0000-0000-0000000000c0', 'en', 'A1', 'English for Beginners',
   'Start speaking English from day one.', 1, true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO units (id, course_id, position, title, description) VALUES
  ('00000000-0000-0000-0000-0000000000d1', '00000000-0000-0000-0000-0000000000c0', 1, 'Getting started', 'Your first words in English.')
ON CONFLICT (id) DO NOTHING;

INSERT INTO lessons (id, course_id, unit_id, position, title, version) VALUES
  ('00000000-0000-0000-0000-0000000000a1', '00000000-0000-0000-0000-0000000000c0', '00000000-0000-0000-0000-0000000000d1', 1, 'Greetings', 1),
  ('00000000-0000-0000-0000-0000000000a2', '00000000-0000-0000-0000-0000000000c0', '00000000-0000-0000-0000-0000000000d1', 2, 'Introductions', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO lesson_items (id, lesson_id, position, kind, payload) VALUES
  ('00000000-0000-0000-0000-0000000000b0', '00000000-0000-0000-0000-0000000000a1', 1, 'image',
   '{"url":"data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijg2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+SGVsbG8hPC90ZXh0Pgo8dGV4dCB4PSI2MiIgeT0iMjYwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIzNCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC44NSI+SG93IGFyZSB5b3U/PC90ZXh0Pgo8Y2lyY2xlIGN4PSI2NjAiIGN5PSIxNTAiIHI9IjkwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEyIi8+CjxjaXJjbGUgY3g9IjcyMCIgY3k9IjMwMCIgcj0iNTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz4KPC9zdmc+","caption":"Greetings"}'),
  ('00000000-0000-0000-0000-0000000000b1', '00000000-0000-0000-0000-0000000000a1', 2, 'text',
   '{"phrase":"Hello! How are you?","translation":{"fa":"سلام! حالت چطوره؟","ckb":"سڵاو! چۆنیت؟"}}'),
  ('00000000-0000-0000-0000-0000000000b2', '00000000-0000-0000-0000-0000000000a1', 3, 'mini_story',
   '{"narrator":"A man meets his friend.","prompt":"What does he say?"}'),
  ('00000000-0000-0000-0000-0000000000b3', '00000000-0000-0000-0000-0000000000a1', 4, 'exercise', '{}'),
  ('00000000-0000-0000-0000-0000000000b4', '00000000-0000-0000-0000-0000000000a2', 1, 'text',
   '{"phrase":"My name is Sara.","translation":{"fa":"اسم من سارا است.","ckb":"ناوم سارایە."}}')
ON CONFLICT (id) DO NOTHING;

INSERT INTO exercises (id, lesson_item_id, type, prompt, answer_key, difficulty) VALUES
  ('00000000-0000-0000-0000-0000000000d1', '00000000-0000-0000-0000-0000000000b3', 'mcq',
   '{"question":"Choose the greeting","options":["Hello","Apple","Three"]}', '{"correct":0}', 1)
ON CONFLICT (id) DO NOTHING;

COMMIT;
