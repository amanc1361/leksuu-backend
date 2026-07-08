-- 0013 — units (chapters). Lessons now live inside a unit; ordering (position)
-- is per-unit. Mirrors Busuu's course → chapter → lesson hierarchy and keeps the
-- admin manageable as lesson counts grow.
CREATE TABLE IF NOT EXISTS units (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id   uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  position    int  NOT NULL,
  title       text NOT NULL,
  description text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (course_id, position)
);
CREATE INDEX IF NOT EXISTS idx_units_course ON units(course_id, position);

ALTER TABLE lessons ADD COLUMN IF NOT EXISTS unit_id uuid REFERENCES units(id) ON DELETE CASCADE;

-- Backfill: every existing course gets a default unit; its lessons attach to it.
INSERT INTO units (course_id, position, title)
SELECT c.id, 1, 'Getting started'
FROM courses c
WHERE NOT EXISTS (SELECT 1 FROM units u WHERE u.course_id = c.id);

UPDATE lessons l
SET unit_id = (SELECT u.id FROM units u WHERE u.course_id = l.course_id ORDER BY u.position LIMIT 1)
WHERE l.unit_id IS NULL;

-- Position is now per-unit, so move the uniqueness off (course_id, position).
ALTER TABLE lessons DROP CONSTRAINT IF EXISTS lessons_course_id_position_key;
ALTER TABLE lessons ADD CONSTRAINT lessons_unit_id_position_key UNIQUE (unit_id, position);
CREATE INDEX IF NOT EXISTS idx_lessons_unit ON lessons(unit_id, position);
