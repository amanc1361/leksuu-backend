-- Phase 6 demo tutors + a rolling 7-day window of future slots (idempotent).
INSERT INTO tutors (id, name, headline, bio, languages, hourly_gems, rating_avg, rating_count) VALUES
  ('tutor-sara',  'Sara Ahmadi', 'Friendly conversation coach',
     'Native-level English teacher focused on speaking confidence for beginners.', 'en,fa', 300, 4.8, 24),
  ('tutor-dilan', 'Dilan Karim', 'Grammar & pronunciation',
     'Patient tutor for Kurdish and Persian speakers learning English from scratch.', 'en,ckb,fa', 250, 4.6, 11),
  ('tutor-john',  'John Miller', 'Exam & business English',
     'Helps learners prepare for interviews and everyday work conversations.', 'en', 400, 4.9, 37)
ON CONFLICT (id) DO NOTHING;

-- Two slots per day for the next 7 days, at 16:00 and 18:00 UTC, per tutor.
INSERT INTO tutor_slots (id, tutor_id, start_utc, duration_min, booked)
SELECT 'slot-' || t.id || '-' || to_char(slot.start_utc, 'MMDD-HH24'),
       t.id, slot.start_utc, 30, false
FROM (VALUES ('tutor-sara'), ('tutor-dilan'), ('tutor-john')) AS t(id)
CROSS JOIN LATERAL (
  SELECT date_trunc('day', now() AT TIME ZONE 'UTC') + (d || ' days')::interval + (h || ' hours')::interval AS start_utc
  FROM generate_series(1, 7) AS d, (VALUES (16), (18)) AS hrs(h)
) AS slot
ON CONFLICT (id) DO NOTHING;
