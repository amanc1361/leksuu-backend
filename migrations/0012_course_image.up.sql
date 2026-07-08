-- Optional cover image for a course (Busuu-style visual cards). NULL → the UI
-- renders a generated gradient cover, so a missing image is never ugly.
ALTER TABLE courses ADD COLUMN IF NOT EXISTS image_url text;
