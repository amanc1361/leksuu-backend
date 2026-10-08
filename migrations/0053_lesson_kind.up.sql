-- 0053 — checkpoints become a first-class lesson kind.
-- Until now a checkpoint was only a lesson with focus='review', so nothing in
-- the data said "this is the chapter's checkpoint" and the app could not show
-- or protect it as one. kind is 'lesson' (default) or 'checkpoint'.
ALTER TABLE lessons ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'lesson';
ALTER TABLE lessons DROP CONSTRAINT IF EXISTS lessons_kind_check;
ALTER TABLE lessons ADD CONSTRAINT lessons_kind_check CHECK (kind IN ('lesson', 'checkpoint'));

-- Existing checkpoints: imported review lessons titled "Checkpoint".
UPDATE lessons
   SET kind = 'checkpoint'
 WHERE kind = 'lesson'
   AND focus = 'review'
   AND title ILIKE 'checkpoint%';
