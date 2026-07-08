-- Bookings now charge the tutor's hourly_gems prorated by duration. The exact
-- amount paid is stored so a cancellation refunds precisely what was charged
-- (even if the tutor later changes their price).
ALTER TABLE tutor_bookings ADD COLUMN IF NOT EXISTS cost_gems int NOT NULL DEFAULT 0;
