-- Per-user custom daily XP goal (0 = use the system default).
ALTER TABLE user_gamification ADD COLUMN daily_target_xp int NOT NULL DEFAULT 0;
