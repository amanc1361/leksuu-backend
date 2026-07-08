-- Phase 6: live tutoring marketplace. All times stored in UTC.
CREATE TABLE IF NOT EXISTS tutors (
  id           text PRIMARY KEY,
  name         text NOT NULL,
  headline     text NOT NULL DEFAULT '',
  bio          text NOT NULL DEFAULT '',
  languages    text NOT NULL DEFAULT '',   -- comma-separated language codes
  avatar_url   text NOT NULL DEFAULT '',
  hourly_gems  int  NOT NULL DEFAULT 0,
  rating_avg   double precision NOT NULL DEFAULT 0,
  rating_count int  NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS tutor_slots (
  id           text PRIMARY KEY,
  tutor_id     text NOT NULL REFERENCES tutors(id) ON DELETE CASCADE,
  start_utc    timestamptz NOT NULL,
  duration_min int  NOT NULL DEFAULT 30,
  booked       boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS idx_tutor_slots_tutor ON tutor_slots(tutor_id, start_utc);
CREATE TABLE IF NOT EXISTS tutor_bookings (
  id           text PRIMARY KEY,
  tutor_id     text NOT NULL REFERENCES tutors(id) ON DELETE CASCADE,
  learner_id   text NOT NULL,
  slot_id      text NOT NULL,
  start_utc    timestamptz NOT NULL,
  duration_min int  NOT NULL DEFAULT 30,
  status       text NOT NULL DEFAULT 'confirmed',
  room_id      text NOT NULL DEFAULT '',
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tutor_bookings_learner ON tutor_bookings(learner_id, start_utc DESC);
CREATE TABLE IF NOT EXISTS tutor_reviews (
  id          text PRIMARY KEY,
  booking_id  text NOT NULL UNIQUE,
  tutor_id    text NOT NULL REFERENCES tutors(id) ON DELETE CASCADE,
  learner_id  text NOT NULL,
  rating      int  NOT NULL,
  comment     text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);
