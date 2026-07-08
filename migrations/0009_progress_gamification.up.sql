-- 0009 — progress (partitioned) + gamification (Duolingo/Expert-style).
CREATE TABLE user_progress (
  id          uuid DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL,
  entity_type text NOT NULL,                    -- 'lesson','exercise','podcast','mini_story','story'
  entity_id   uuid NOT NULL,
  status      text NOT NULL,                    -- 'started','completed'
  score       real,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);
CREATE INDEX idx_progress_user_time ON user_progress(user_id, occurred_at DESC);

CREATE TABLE user_gamification (
  user_id        uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  xp             int  NOT NULL DEFAULT 0,
  gems           int  NOT NULL DEFAULT 0,
  streak_days    int  NOT NULL DEFAULT 0,
  longest_streak int  NOT NULL DEFAULT 0,
  last_active    date,
  league         text NOT NULL DEFAULT 'bronze',
  updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE daily_goals (
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  goal_date   date NOT NULL,
  target_xp   int  NOT NULL DEFAULT 50,
  earned_xp   int  NOT NULL DEFAULT 0,
  met         boolean NOT NULL DEFAULT false,
  PRIMARY KEY (user_id, goal_date)
);

CREATE TABLE achievements (
  id          text PRIMARY KEY,                 -- 'streak_7','words_100'
  title       jsonb NOT NULL,                   -- localized
  icon        text
);

CREATE TABLE user_achievements (
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  achievement_id text NOT NULL REFERENCES achievements(id),
  earned_at      timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, achievement_id)
);
