-- Busuu-style lesson presentation: a cover image, a focus tag, and a localized
-- "what you will learn" objective shown as the lesson's intro screen.
ALTER TABLE lessons ADD COLUMN IF NOT EXISTS cover_url text;
ALTER TABLE lessons ADD COLUMN IF NOT EXISTS focus     text;
ALTER TABLE lessons ADD COLUMN IF NOT EXISTS objective jsonb;
