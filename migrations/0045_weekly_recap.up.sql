-- Weekly recap notifications: one per user per completed ISO week.
CREATE TABLE IF NOT EXISTS weekly_recaps (
    user_id  text NOT NULL,
    week_key text NOT NULL,
    sent_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, week_key)
);
