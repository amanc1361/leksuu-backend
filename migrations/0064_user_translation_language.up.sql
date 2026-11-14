-- Keep the learner's media translation choice with the account profile.
-- Existing Sorani users receive Sorani; all other accounts default to Persian.
ALTER TABLE users ADD COLUMN translation_lang text NOT NULL DEFAULT 'fa'
  CONSTRAINT users_translation_lang_check CHECK (translation_lang IN ('fa', 'ckb'));
UPDATE users SET translation_lang = 'ckb' WHERE native_lang = 'ckb';
