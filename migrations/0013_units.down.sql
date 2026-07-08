-- Rollback: collapse positions back to per-course uniqueness, then drop units.
ALTER TABLE lessons DROP CONSTRAINT IF EXISTS lessons_unit_id_position_key;

-- Re-number lesson positions to be unique within each course (units may have
-- introduced duplicate per-course positions).
WITH ranked AS (
  SELECT l.id,
         row_number() OVER (
           PARTITION BY l.course_id
           ORDER BY COALESCE((SELECT u.position FROM units u WHERE u.id = l.unit_id), 0), l.position, l.id
         ) AS rn
  FROM lessons l
)
UPDATE lessons l SET position = r.rn FROM ranked r WHERE l.id = r.id;

ALTER TABLE lessons ADD CONSTRAINT lessons_course_id_position_key UNIQUE (course_id, position);
ALTER TABLE lessons DROP COLUMN IF EXISTS unit_id;
DROP TABLE IF EXISTS units;
