-- 0050 — rebuild empty imported checkpoints (QA N08).
-- Busuu checkpoints arrived with no convertible nodes, so 36 imported review
-- lessons had zero items and opened as an empty, completable page. Rebuild each
-- one from its own chapter: up to two graded exercise steps per sibling lesson
-- (spread across lessons first), at most 12, copied with their full prompt and
-- answer key. Copies are tagged source_ref='checkpoint-backfill' so a later
-- re-import replaces them like any other imported item. Idempotent: only
-- lessons that still have no items are touched.
DO $$
DECLARE
  cp       RECORD;
  src      RECORD;
  new_item uuid;
  pos      int;
BEGIN
  FOR cp IN
    SELECT l.id, l.unit_id
      FROM lessons l
     WHERE l.focus = 'review'
       AND l.source_id IS NOT NULL AND l.source_id <> ''
       AND l.unit_id IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM lesson_items li WHERE li.lesson_id = l.id)
  LOOP
    pos := 0;
    FOR src IN
      SELECT ranked.id, ranked.kind, ranked.payload, ranked.media_key
        FROM (
          SELECT li.id, li.kind, li.payload, li.media_key, l2.position AS lpos,
                 row_number() OVER (PARTITION BY li.lesson_id ORDER BY li.position) AS rn
            FROM lesson_items li
            JOIN lessons l2 ON l2.id = li.lesson_id
           WHERE l2.unit_id = cp.unit_id
             AND l2.id <> cp.id
             AND li.kind = 'exercise'
             AND EXISTS (SELECT 1 FROM exercises e WHERE e.lesson_item_id = li.id)
        ) ranked
       WHERE ranked.rn <= 2
       ORDER BY ranked.rn, ranked.lpos
       LIMIT 12
    LOOP
      pos := pos + 1;
      INSERT INTO lesson_items (lesson_id, position, kind, payload, media_key, source_ref)
      VALUES (cp.id, pos, src.kind, src.payload, src.media_key, 'checkpoint-backfill')
      RETURNING id INTO new_item;
      INSERT INTO exercises (lesson_item_id, type, prompt, answer_key, difficulty)
      SELECT new_item, e.type, e.prompt, e.answer_key, e.difficulty
        FROM exercises e
       WHERE e.lesson_item_id = src.id;
    END LOOP;
  END LOOP;
END $$;
