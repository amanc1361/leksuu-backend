-- Shadowing practice: per-user repetition counts per media-item transcript line.
CREATE TABLE shadowing_progress (
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  item_id    uuid NOT NULL REFERENCES lesson_items(id) ON DELETE CASCADE,
  reps       jsonb NOT NULL DEFAULT '{}',
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, item_id)
);
