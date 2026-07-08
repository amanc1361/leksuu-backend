-- A1 vocabulary seed for the English course (dictionary + SRS source).
-- Idempotent on the natural key (language, item_kind, lemma).
BEGIN;

INSERT INTO vocabulary_items (language, item_kind, lemma, pronunciation, meaning, example) VALUES
  ('en', 'word', 'hello', '/həˈloʊ/', '{"fa": "سلام", "ckb": "سڵاو"}', 'Hello, how are you?'),
  ('en', 'word', 'name', '/neɪm/', '{"fa": "نام", "ckb": "ناو"}', 'My name is Sara.'),
  ('en', 'word', 'mother', '/ˈmʌðər/', '{"fa": "مادر", "ckb": "دایک"}', 'This is my mother.'),
  ('en', 'word', 'father', '/ˈfɑːðər/', '{"fa": "پدر", "ckb": "باوک"}', 'He is my father.'),
  ('en', 'word', 'brother', '/ˈbrʌðər/', '{"fa": "برادر", "ckb": "برا"}', 'He is my brother.'),
  ('en', 'word', 'sister', '/ˈsɪstər/', '{"fa": "خواهر", "ckb": "خوشک"}', 'She is my sister.'),
  ('en', 'word', 'family', '/ˈfæməli/', '{"fa": "خانواده", "ckb": "خێزان"}', 'I have a small family.'),
  ('en', 'word', 'one', '/wʌn/', '{"fa": "یک", "ckb": "یەک"}', 'I have one book.'),
  ('en', 'word', 'two', '/tuː/', '{"fa": "دو", "ckb": "دوو"}', 'I have two books.'),
  ('en', 'word', 'three', '/θriː/', '{"fa": "سه", "ckb": "سێ"}', 'Three apples.'),
  ('en', 'word', 'four', '/fɔːr/', '{"fa": "چهار", "ckb": "چوار"}', 'Four cats.'),
  ('en', 'word', 'five', '/faɪv/', '{"fa": "پنج", "ckb": "پێنج"}', 'It is five dollars.'),
  ('en', 'word', 'book', '/bʊk/', '{"fa": "کتاب", "ckb": "پەرتووک"}', 'I read a book.'),
  ('en', 'word', 'apple', '/ˈæpəl/', '{"fa": "سیب", "ckb": "سێو"}', 'The apple is red.'),
  ('en', 'word', 'coffee', '/ˈkɔːfi/', '{"fa": "قهوه", "ckb": "قاوە"}', 'I like coffee.'),
  ('en', 'word', 'water', '/ˈwɔːtər/', '{"fa": "آب", "ckb": "ئاو"}', 'She wants water.'),
  ('en', 'word', 'tea', '/tiː/', '{"fa": "چای", "ckb": "چا"}', 'I like tea.'),
  ('en', 'word', 'bread', '/bred/', '{"fa": "نان", "ckb": "نان"}', 'She buys bread.'),
  ('en', 'word', 'milk', '/mɪlk/', '{"fa": "شیر", "ckb": "شیر"}', 'She buys milk.'),
  ('en', 'word', 'red', '/red/', '{"fa": "قرمز", "ckb": "سوور"}', 'The apple is red.'),
  ('en', 'word', 'blue', '/bluː/', '{"fa": "آبی", "ckb": "شین"}', 'The sky is blue.'),
  ('en', 'word', 'green', '/ɡriːn/', '{"fa": "سبز", "ckb": "سەوز"}', 'Grass is green.'),
  ('en', 'word', 'big', '/bɪɡ/', '{"fa": "بزرگ", "ckb": "گەورە"}', 'The elephant is big.'),
  ('en', 'word', 'small', '/smɔːl/', '{"fa": "کوچک", "ckb": "بچووک"}', 'The mouse is small.'),
  ('en', 'word', 'time', '/taɪm/', '{"fa": "زمان", "ckb": "کات"}', 'What time is it?'),
  ('en', 'word', 'hour', '/ˈaʊər/', '{"fa": "ساعت", "ckb": "کاتژمێر"}', 'One hour.'),
  ('en', 'word', 'money', '/ˈmʌni/', '{"fa": "پول", "ckb": "پارە"}', 'I have money.'),
  ('en', 'word', 'shop', '/ʃɒp/', '{"fa": "مغازه", "ckb": "دوکان"}', 'She goes to the shop.'),
  ('en', 'word', 'left', '/left/', '{"fa": "چپ", "ckb": "چەپ"}', 'Turn left.'),
  ('en', 'word', 'right', '/raɪt/', '{"fa": "راست", "ckb": "ڕاست"}', 'The shop is on the right.'),
  ('en', 'word', 'work', '/wɜːrk/', '{"fa": "کار", "ckb": "کار"}', 'She goes to work.'),
  ('en', 'word', 'like', '/laɪk/', '{"fa": "دوست داشتن", "ckb": "حەزکردن"}', 'I like coffee.')
ON CONFLICT (language, item_kind, lemma) DO NOTHING;

COMMIT;
