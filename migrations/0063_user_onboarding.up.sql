-- Persist one-time onboarding on the account, not a browser. Backfill all
-- pre-existing accounts as completed, so an upgrade never re-onboards learners.
-- Newly created accounts after migration have no row until they complete/skip.
CREATE TABLE user_onboarding (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  completed_at timestamptz NOT NULL DEFAULT now(),
  skipped boolean NOT NULL DEFAULT false,
  answers jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(answers) = 'object')
);

INSERT INTO user_onboarding(user_id, completed_at, skipped, answers)
SELECT id, now(), false, '{}'::jsonb FROM users
ON CONFLICT (user_id) DO NOTHING;
