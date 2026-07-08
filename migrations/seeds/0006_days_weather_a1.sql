-- A1 unit 5: Days and Weather + its vocabulary (idempotent).
BEGIN;

INSERT INTO units (id, course_id, position, title, description) VALUES
  ('00000000-0000-0000-0000-000000240000', '00000000-0000-0000-0000-0000000000c0', 5, 'Days and Weather', 'Days of the week and the weather.')
ON CONFLICT (id) DO NOTHING;

INSERT INTO lessons (id, course_id, unit_id, position, title, version) VALUES
  ('00000000-0000-0000-0000-000000240001', '00000000-0000-0000-0000-0000000000c0', '00000000-0000-0000-0000-000000240000', 1, 'Days of the Week', 1),
  ('00000000-0000-0000-0000-000000240002', '00000000-0000-0000-0000-0000000000c0', '00000000-0000-0000-0000-000000240000', 2, 'Weather', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO lesson_items (id, lesson_id, position, kind, payload) VALUES
  ('00000000-0000-0000-0000-000000240003', '00000000-0000-0000-0000-000000240001', 1, 'image', '{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjYjQ1MDNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5EYXlzPC90ZXh0Pjx0ZXh0IHg9IjYyIiB5PSIyNTgiIGZvbnQtZmFtaWx5PSJzYW5zLXNlcmlmIiBmb250LXNpemU9IjMyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjg1Ij5Nb25kYXkgdG8gU3VuZGF5PC90ZXh0PjxjaXJjbGUgY3g9IjY2MCIgY3k9IjE1MCIgcj0iOTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTIiLz48Y2lyY2xlIGN4PSI3MjAiIGN5PSIzMDAiIHI9IjUwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEwIi8+PC9zdmc+", "caption": "Days"}'),
  ('00000000-0000-0000-0000-000000240004', '00000000-0000-0000-0000-000000240001', 2, 'text', '{"phrase": "Today is Monday.", "translation": {"fa": "امروز دوشنبه است.", "ckb": "ئەمڕۆ دووشەممەیە."}}'),
  ('00000000-0000-0000-0000-000000240005', '00000000-0000-0000-0000-000000240001', 3, 'text', '{"phrase": "Tomorrow is Tuesday.", "translation": {"fa": "فردا سه‌شنبه است.", "ckb": "سبەینێ سێشەممەیە."}}'),
  ('00000000-0000-0000-0000-000000240006', '00000000-0000-0000-0000-000000240001', 4, 'exercise', '{}'),
  ('00000000-0000-0000-0000-00000024000a', '00000000-0000-0000-0000-000000240002', 1, 'image', '{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjYjQ1MDNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5XZWF0aGVyPC90ZXh0Pjx0ZXh0IHg9IjYyIiB5PSIyNTgiIGZvbnQtZmFtaWx5PSJzYW5zLXNlcmlmIiBmb250LXNpemU9IjMyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjg1Ij5TdW4sIHJhaW4sIHNub3c8L3RleHQ+PGNpcmNsZSBjeD0iNjYwIiBjeT0iMTUwIiByPSI5MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMiIvPjxjaXJjbGUgY3g9IjcyMCIgY3k9IjMwMCIgcj0iNTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48L3N2Zz4=", "caption": "Weather"}'),
  ('00000000-0000-0000-0000-00000024000b', '00000000-0000-0000-0000-000000240002', 2, 'text', '{"phrase": "It is hot today.", "translation": {"fa": "امروز گرم است.", "ckb": "ئەمڕۆ گەرمە."}}'),
  ('00000000-0000-0000-0000-00000024000c', '00000000-0000-0000-0000-000000240002', 3, 'text', '{"phrase": "It is cold in winter.", "translation": {"fa": "در زمستان سرد است.", "ckb": "لە زستاندا ساردە."}}'),
  ('00000000-0000-0000-0000-00000024000d', '00000000-0000-0000-0000-000000240002', 4, 'mini_story', '{"narrator": "Today it is rainy. Sara takes her umbrella and goes to work.", "prompt": "What does Sara take?", "translation": {"fa": "امروز بارانی است. سارا چترش را برمی‌دارد و به سرِ کار می‌رود.", "ckb": "ئەمڕۆ بارانییە. سارا چەترەکەی هەڵدەگرێت و دەچێتە سەر کار."}}'),
  ('00000000-0000-0000-0000-00000024000e', '00000000-0000-0000-0000-000000240002', 5, 'exercise', '{}')
ON CONFLICT (id) DO NOTHING;

INSERT INTO exercises (id, lesson_item_id, type, prompt, answer_key, difficulty) VALUES
  ('00000000-0000-0000-0000-000000240007', '00000000-0000-0000-0000-000000240006', 'mcq', '{"question": "Which one is a day of the week?", "options": ["Monday", "Apple", "Blue"]}', '{"correct": 0}', 1),
  ('00000000-0000-0000-0000-000000240008', '00000000-0000-0000-0000-000000240006', 'reorder', '{"words": ["Today", "is", "Monday"]}', '{"order": ["Today", "is", "Monday"]}', 1),
  ('00000000-0000-0000-0000-000000240009', '00000000-0000-0000-0000-000000240006', 'fill_blank', '{"text": "There are seven days in a ____ . (week)"}', '{"answers": ["week"]}', 1),
  ('00000000-0000-0000-0000-00000024000f', '00000000-0000-0000-0000-00000024000e', 'mcq', '{"question": "What does Sara take?", "options": ["an umbrella", "a book", "a dog"]}', '{"correct": 0}', 1),
  ('00000000-0000-0000-0000-000000240010', '00000000-0000-0000-0000-00000024000e', 'true_false', '{"statement": "It is hot in winter."}', '{"answer": false}', 1),
  ('00000000-0000-0000-0000-000000240011', '00000000-0000-0000-0000-00000024000e', 'match', '{"left": ["hot", "cold", "rain"], "right": ["گرم", "سرد", "باران"]}', '{"pairs": [["hot", "گرم"], ["cold", "سرد"], ["rain", "باران"]]}', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vocabulary_items (language, item_kind, lemma, pronunciation, meaning, example) VALUES
  ('en', 'word', 'monday', '/ˈmʌndeɪ/', '{"fa": "دوشنبه", "ckb": "دووشەممە"}', 'Today is Monday.'),
  ('en', 'word', 'today', '/təˈdeɪ/', '{"fa": "امروز", "ckb": "ئەمڕۆ"}', 'It is hot today.'),
  ('en', 'word', 'tomorrow', '/təˈmɒroʊ/', '{"fa": "فردا", "ckb": "سبەینێ"}', 'Tomorrow is Tuesday.'),
  ('en', 'word', 'week', '/wiːk/', '{"fa": "هفته", "ckb": "هەفتە"}', 'There are seven days in a week.'),
  ('en', 'word', 'day', '/deɪ/', '{"fa": "روز", "ckb": "ڕۆژ"}', 'It is a sunny day.'),
  ('en', 'word', 'hot', '/hɒt/', '{"fa": "گرم", "ckb": "گەرم"}', 'It is hot today.'),
  ('en', 'word', 'cold', '/koʊld/', '{"fa": "سرد", "ckb": "سارد"}', 'It is cold in winter.'),
  ('en', 'word', 'rain', '/reɪn/', '{"fa": "باران", "ckb": "باران"}', 'I like the rain.'),
  ('en', 'word', 'sun', '/sʌn/', '{"fa": "خورشید", "ckb": "خۆر"}', 'The sun is hot.'),
  ('en', 'word', 'snow', '/snoʊ/', '{"fa": "برف", "ckb": "بەفر"}', 'Snow is white.'),
  ('en', 'word', 'umbrella', '/ʌmˈbrelə/', '{"fa": "چتر", "ckb": "چەتر"}', 'She takes her umbrella.'),
  ('en', 'word', 'winter', '/ˈwɪntər/', '{"fa": "زمستان", "ckb": "زستان"}', 'It is cold in winter.')
ON CONFLICT (language, item_kind, lemma) DO NOTHING;

COMMIT;
