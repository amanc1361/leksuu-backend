-- A1 'Everyday Life' unit + extra Introductions exercises (idempotent).
-- Generated; safe to re-run (fixed UUIDs + ON CONFLICT DO NOTHING).
BEGIN;

INSERT INTO units (id, course_id, position, title, description) VALUES
  ('00000000-0000-0000-0000-000000210000', '00000000-0000-0000-0000-0000000000c0', 2, 'Everyday Life', 'Words and phrases for daily life.')
ON CONFLICT (id) DO NOTHING;

INSERT INTO lessons (id, course_id, unit_id, position, title, version) VALUES
  ('00000000-0000-0000-0000-000000210001', '00000000-0000-0000-0000-0000000000c0', '00000000-0000-0000-0000-000000210000', 1, 'Numbers 1 to 5', 1),
  ('00000000-0000-0000-0000-000000210002', '00000000-0000-0000-0000-0000000000c0', '00000000-0000-0000-0000-000000210000', 2, 'Family', 1),
  ('00000000-0000-0000-0000-000000210003', '00000000-0000-0000-0000-0000000000c0', '00000000-0000-0000-0000-000000210000', 3, 'Food and Drink', 1),
  ('00000000-0000-0000-0000-000000210004', '00000000-0000-0000-0000-0000000000c0', '00000000-0000-0000-0000-000000210000', 4, 'Daily Routine', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO lesson_items (id, lesson_id, position, kind, payload) VALUES
  ('00000000-0000-0000-0000-000000210005', '00000000-0000-0000-0000-000000210001', 1, 'image', '{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijc2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+TnVtYmVyczwvdGV4dD4KPHRleHQgeD0iNjIiIHk9IjI1OCIgZm9udC1mYW1pbHk9InNhbnMtc2VyaWYiIGZvbnQtc2l6ZT0iMzIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuODUiPk9uZSwgdHdvLCB0aHJlZTwvdGV4dD4KPGNpcmNsZSBjeD0iNjYwIiBjeT0iMTUwIiByPSI5MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMiIvPgo8Y2lyY2xlIGN4PSI3MjAiIGN5PSIzMDAiIHI9IjUwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEwIi8+PC9zdmc+", "caption": "Numbers"}'),
  ('00000000-0000-0000-0000-000000210006', '00000000-0000-0000-0000-000000210001', 2, 'text', '{"phrase": "One, two, three.", "translation": {"fa": "یک، دو، سه.", "ckb": "یەک، دوو، سێ."}}'),
  ('00000000-0000-0000-0000-000000210007', '00000000-0000-0000-0000-000000210001', 3, 'text', '{"phrase": "I have two books.", "translation": {"fa": "من دو کتاب دارم.", "ckb": "دوو پەرتووکم هەیە."}}'),
  ('00000000-0000-0000-0000-000000210008', '00000000-0000-0000-0000-000000210001', 4, 'exercise', '{}'),
  ('00000000-0000-0000-0000-00000021000c', '00000000-0000-0000-0000-000000210002', 1, 'image', '{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijc2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+RmFtaWx5PC90ZXh0Pgo8dGV4dCB4PSI2MiIgeT0iMjU4IiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIzMiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC44NSI+TW90aGVyLCBmYXRoZXIsIGJyb3RoZXI8L3RleHQ+CjxjaXJjbGUgY3g9IjY2MCIgY3k9IjE1MCIgcj0iOTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTIiLz4KPGNpcmNsZSBjeD0iNzIwIiBjeT0iMzAwIiByPSI1MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjwvc3ZnPg==", "caption": "Family"}'),
  ('00000000-0000-0000-0000-00000021000d', '00000000-0000-0000-0000-000000210002', 2, 'text', '{"phrase": "This is my mother.", "translation": {"fa": "این مادرِ من است.", "ckb": "ئەمە دایکمە."}}'),
  ('00000000-0000-0000-0000-00000021000e', '00000000-0000-0000-0000-000000210002', 3, 'text', '{"phrase": "He is my brother.", "translation": {"fa": "او برادرِ من است.", "ckb": "ئەو براممە."}}'),
  ('00000000-0000-0000-0000-00000021000f', '00000000-0000-0000-0000-000000210002', 4, 'mini_story', '{"narrator": "Tom has a small family. He has one sister and two brothers.", "prompt": "How many brothers does Tom have?", "translation": {"fa": "تام خانواده‌ی کوچکی دارد. او یک خواهر و دو برادر دارد.", "ckb": "تۆم خێزانێکی بچووکی هەیە. خوشکێک و دوو برای هەیە."}}'),
  ('00000000-0000-0000-0000-000000210010', '00000000-0000-0000-0000-000000210002', 5, 'exercise', '{}'),
  ('00000000-0000-0000-0000-000000210014', '00000000-0000-0000-0000-000000210003', 1, 'image', '{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijc2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+Rm9vZCAmIERyaW5rPC90ZXh0Pgo8dGV4dCB4PSI2MiIgeT0iMjU4IiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIzMiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC44NSI+Q29mZmVlLCB3YXRlciwgYnJlYWQ8L3RleHQ+CjxjaXJjbGUgY3g9IjY2MCIgY3k9IjE1MCIgcj0iOTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTIiLz4KPGNpcmNsZSBjeD0iNzIwIiBjeT0iMzAwIiByPSI1MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjwvc3ZnPg==", "caption": "Food & Drink"}'),
  ('00000000-0000-0000-0000-000000210015', '00000000-0000-0000-0000-000000210003', 2, 'text', '{"phrase": "I like coffee.", "translation": {"fa": "من قهوه دوست دارم.", "ckb": "حەز لە قاوە دەکەم."}}'),
  ('00000000-0000-0000-0000-000000210016', '00000000-0000-0000-0000-000000210003', 3, 'text', '{"phrase": "She wants water, please.", "translation": {"fa": "او آب می‌خواهد، لطفا.", "ckb": "ئەو ئاوی دەوێت، تکایە."}}'),
  ('00000000-0000-0000-0000-000000210017', '00000000-0000-0000-0000-000000210003', 4, 'exercise', '{}'),
  ('00000000-0000-0000-0000-00000021001b', '00000000-0000-0000-0000-000000210004', 1, 'image', '{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijc2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+RGFpbHkgUm91dGluZTwvdGV4dD4KPHRleHQgeD0iNjIiIHk9IjI1OCIgZm9udC1mYW1pbHk9InNhbnMtc2VyaWYiIGZvbnQtc2l6ZT0iMzIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuODUiPldha2UgdXAsIHdvcmssIHNsZWVwPC90ZXh0Pgo8Y2lyY2xlIGN4PSI2NjAiIGN5PSIxNTAiIHI9IjkwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEyIi8+CjxjaXJjbGUgY3g9IjcyMCIgY3k9IjMwMCIgcj0iNTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48L3N2Zz4=", "caption": "Daily Routine"}'),
  ('00000000-0000-0000-0000-00000021001c', '00000000-0000-0000-0000-000000210004', 2, 'text', '{"phrase": "I wake up at seven.", "translation": {"fa": "من ساعتِ هفت بیدار می‌شوم.", "ckb": "لە کاتژمێر حەوت لە خەو هەڵدەستم."}}'),
  ('00000000-0000-0000-0000-00000021001d', '00000000-0000-0000-0000-000000210004', 3, 'text', '{"phrase": "She goes to work.", "translation": {"fa": "او به سرِ کار می‌رود.", "ckb": "ئەو دەچێتە سەر کار."}}'),
  ('00000000-0000-0000-0000-00000021001e', '00000000-0000-0000-0000-000000210004', 4, 'mini_story', '{"narrator": "Every day, Sara wakes up at six. She drinks coffee and goes to work.", "prompt": "What does Sara drink?", "translation": {"fa": "هر روز، سارا ساعتِ شش بیدار می‌شود. او قهوه می‌نوشد و به سرِ کار می‌رود.", "ckb": "هەموو ڕۆژێک، سارا لە شەش لە خەو هەڵدەستێت. قاوە دەخواتەوە و دەچێتە سەر کار."}}'),
  ('00000000-0000-0000-0000-00000021001f', '00000000-0000-0000-0000-000000210004', 5, 'exercise', '{}')
ON CONFLICT (id) DO NOTHING;

INSERT INTO exercises (id, lesson_item_id, type, prompt, answer_key, difficulty) VALUES
  ('00000000-0000-0000-0000-000000210009', '00000000-0000-0000-0000-000000210008', 'mcq', '{"question": "What is the number 3?", "options": ["three", "five", "eight"]}', '{"correct": 0}', 1),
  ('00000000-0000-0000-0000-00000021000a', '00000000-0000-0000-0000-000000210008', 'match', '{"left": ["1", "2", "3"], "right": ["one", "two", "three"]}', '{"pairs": [["1", "one"], ["2", "two"], ["3", "three"]]}', 1),
  ('00000000-0000-0000-0000-00000021000b', '00000000-0000-0000-0000-000000210008', 'fill_blank', '{"text": "I have ____ apples. (2)"}', '{"answers": ["two", "2"]}', 1),
  ('00000000-0000-0000-0000-000000210011', '00000000-0000-0000-0000-000000210010', 'mcq', '{"question": "How many brothers does Tom have?", "options": ["two", "one", "three"]}', '{"correct": 0}', 1),
  ('00000000-0000-0000-0000-000000210012', '00000000-0000-0000-0000-000000210010', 'true_false', '{"statement": "Tom has one sister."}', '{"answer": true}', 1),
  ('00000000-0000-0000-0000-000000210013', '00000000-0000-0000-0000-000000210010', 'reorder', '{"words": ["This", "is", "my", "father"]}', '{"order": ["This", "is", "my", "father"]}', 1),
  ('00000000-0000-0000-0000-000000210018', '00000000-0000-0000-0000-000000210017', 'mcq', '{"question": "Choose a drink.", "options": ["coffee", "table", "chair"]}', '{"correct": 0}', 1),
  ('00000000-0000-0000-0000-000000210019', '00000000-0000-0000-0000-000000210017', 'fill_blank', '{"text": "I ____ tea. (like)"}', '{"answers": ["like"]}', 1),
  ('00000000-0000-0000-0000-00000021001a', '00000000-0000-0000-0000-000000210017', 'true_false', '{"statement": "Water is a drink."}', '{"answer": true}', 1),
  ('00000000-0000-0000-0000-000000210020', '00000000-0000-0000-0000-00000021001f', 'mcq', '{"question": "What does Sara drink?", "options": ["coffee", "milk", "juice"]}', '{"correct": 0}', 1),
  ('00000000-0000-0000-0000-000000210021', '00000000-0000-0000-0000-00000021001f', 'sentence_builder', '{"words": ["She", "goes", "to", "work"]}', '{"order": ["She", "goes", "to", "work"]}', 1),
  ('00000000-0000-0000-0000-000000210022', '00000000-0000-0000-0000-00000021001f', 'true_false', '{"statement": "Sara wakes up at six."}', '{"answer": true}', 1)
ON CONFLICT (id) DO NOTHING;

-- enrich Introductions with an exercise block
INSERT INTO lesson_items (id, lesson_id, position, kind, payload) VALUES
  ('00000000-0000-0000-0000-000000210023', '00000000-0000-0000-0000-0000000000a2', 2, 'exercise', '{}')
ON CONFLICT (id) DO NOTHING;

INSERT INTO exercises (id, lesson_item_id, type, prompt, answer_key, difficulty) VALUES
  ('00000000-0000-0000-0000-000000210024', '00000000-0000-0000-0000-000000210023', 'mcq', '{"question": "Choose the introduction.", "options": ["My name is Ali.", "Red apple.", "Five dogs."]}', '{"correct": 0}', 1),
  ('00000000-0000-0000-0000-000000210025', '00000000-0000-0000-0000-000000210023', 'fill_blank', '{"text": "My ____ is Sara. (name)"}', '{"answers": ["name"]}', 1),
  ('00000000-0000-0000-0000-000000210026', '00000000-0000-0000-0000-000000210023', 'reorder', '{"words": ["I", "am", "from", "Iran"]}', '{"order": ["I", "am", "from", "Iran"]}', 1)
ON CONFLICT (id) DO NOTHING;

COMMIT;
