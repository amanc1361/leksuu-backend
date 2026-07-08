//go:build pgx

package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
)

type PostgresTutorRepo struct{ pool *pgxpool.Pool }

func NewPostgresTutorRepo(pool *pgxpool.Pool) *PostgresTutorRepo {
	return &PostgresTutorRepo{pool: pool}
}

func splitLangs(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (r *PostgresTutorRepo) scanTutor(row interface {
	Scan(dest ...any) error
}) (*domain.Tutor, error) {
	var t domain.Tutor
	var langs string
	if err := row.Scan(&t.ID, &t.Name, &t.Headline, &t.Bio, &langs, &t.AvatarURL, &t.HourlyGems, &t.RatingAvg, &t.RatingCount, &t.UserID, &t.Status); err != nil {
		return nil, err
	}
	t.Languages = splitLangs(langs)
	return &t, nil
}

func (r *PostgresTutorRepo) ListTutors(ctx context.Context, lang string) ([]domain.Tutor, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, headline, bio, languages, avatar_url, hourly_gems, rating_avg, rating_count, user_id, status
		   FROM tutors ORDER BY rating_avg DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Tutor{}
	for rows.Next() {
		t, err := r.scanTutor(rows)
		if err != nil {
			return nil, err
		}
		if lang != "" {
			ok := false
			for _, l := range t.Languages {
				if l == lang {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *PostgresTutorRepo) Tutor(ctx context.Context, id string) (*domain.Tutor, error) {
	t, err := r.scanTutor(r.pool.QueryRow(ctx,
		`SELECT id, name, headline, bio, languages, avatar_url, hourly_gems, rating_avg, rating_count, user_id, status
		   FROM tutors WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (r *PostgresTutorRepo) Availability(ctx context.Context, tutorID string, after time.Time) ([]domain.Slot, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tutor_id, start_utc, duration_min, booked
		   FROM tutor_slots WHERE tutor_id=$1 AND booked=false AND start_utc > $2
		   ORDER BY start_utc`, tutorID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Slot{}
	for rows.Next() {
		var s domain.Slot
		if err := rows.Scan(&s.ID, &s.TutorID, &s.StartUTC, &s.DurationMin, &s.Booked); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresTutorRepo) Slot(ctx context.Context, id string) (*domain.Slot, error) {
	var s domain.Slot
	err := r.pool.QueryRow(ctx,
		`SELECT id, tutor_id, start_utc, duration_min, booked FROM tutor_slots WHERE id=$1`, id).
		Scan(&s.ID, &s.TutorID, &s.StartUTC, &s.DurationMin, &s.Booked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresTutorRepo) SetSlotBooked(ctx context.Context, slotID string, booked bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE tutor_slots SET booked=$2 WHERE id=$1`, slotID, booked)
	return err
}

func (r *PostgresTutorRepo) CreateBooking(ctx context.Context, b *domain.Booking) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tutor_bookings (id, tutor_id, learner_id, slot_id, start_utc, duration_min, status, room_id, created_at, cost_gems)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		b.ID, b.TutorID, b.LearnerID, b.SlotID, b.StartUTC, b.DurationMin, b.Status, b.RoomID, b.CreatedAt, b.CostGems)
	return err
}

func (r *PostgresTutorRepo) Booking(ctx context.Context, id string) (*domain.Booking, error) {
	var b domain.Booking
	err := r.pool.QueryRow(ctx,
		`SELECT id, tutor_id, learner_id, slot_id, start_utc, duration_min, status, room_id, created_at, cost_gems
		   FROM tutor_bookings WHERE id=$1`, id).
		Scan(&b.ID, &b.TutorID, &b.LearnerID, &b.SlotID, &b.StartUTC, &b.DurationMin, &b.Status, &b.RoomID, &b.CreatedAt, &b.CostGems)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *PostgresTutorRepo) BookingsByLearner(ctx context.Context, learnerID string) ([]domain.Booking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tutor_id, learner_id, slot_id, start_utc, duration_min, status, room_id, created_at, cost_gems
		   FROM tutor_bookings WHERE learner_id=$1 ORDER BY start_utc DESC`, learnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Booking{}
	for rows.Next() {
		var b domain.Booking
		if err := rows.Scan(&b.ID, &b.TutorID, &b.LearnerID, &b.SlotID, &b.StartUTC, &b.DurationMin, &b.Status, &b.RoomID, &b.CreatedAt, &b.CostGems); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresTutorRepo) UpdateBookingStatus(ctx context.Context, id, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE tutor_bookings SET status=$2 WHERE id=$1`, id, status)
	return err
}

func (r *PostgresTutorRepo) CreateReview(ctx context.Context, rv *domain.Review) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tutor_reviews (id, booking_id, tutor_id, learner_id, rating, comment, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		rv.ID, rv.BookingID, rv.TutorID, rv.LearnerID, rv.Rating, rv.Comment, rv.CreatedAt)
	return err
}

func (r *PostgresTutorRepo) ReviewByBooking(ctx context.Context, bookingID string) (*domain.Review, error) {
	var rv domain.Review
	err := r.pool.QueryRow(ctx,
		`SELECT id, booking_id, tutor_id, learner_id, rating, comment, created_at
		   FROM tutor_reviews WHERE booking_id=$1`, bookingID).
		Scan(&rv.ID, &rv.BookingID, &rv.TutorID, &rv.LearnerID, &rv.Rating, &rv.Comment, &rv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rv, nil
}

func (r *PostgresTutorRepo) RecomputeRating(ctx context.Context, tutorID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tutors t SET
		   rating_count = sub.cnt,
		   rating_avg = round(sub.avg::numeric, 1)
		 FROM (SELECT count(*) cnt, coalesce(avg(rating),0) avg FROM tutor_reviews WHERE tutor_id=$1) sub
		 WHERE t.id=$1`, tutorID)
	return err
}

func (r *PostgresTutorRepo) TutorByUser(ctx context.Context, userID string) (*domain.Tutor, error) {
	if userID == "" {
		return nil, nil
	}
	t, err := r.scanTutor(r.pool.QueryRow(ctx,
		`SELECT id, name, headline, bio, languages, avatar_url, hourly_gems, rating_avg, rating_count, user_id, status
		   FROM tutors WHERE user_id=$1`, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (r *PostgresTutorRepo) BookingsByTutor(ctx context.Context, tutorID string) ([]domain.Booking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tutor_id, learner_id, slot_id, start_utc, duration_min, status, room_id, created_at, cost_gems
		   FROM tutor_bookings WHERE tutor_id=$1 ORDER BY start_utc DESC`, tutorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Booking{}
	for rows.Next() {
		var b domain.Booking
		if err := rows.Scan(&b.ID, &b.TutorID, &b.LearnerID, &b.SlotID, &b.StartUTC, &b.DurationMin, &b.Status, &b.RoomID, &b.CreatedAt, &b.CostGems); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresTutorRepo) LinkUser(ctx context.Context, tutorID, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE tutors SET user_id=$2 WHERE id=$1`, tutorID, userID)
	return err
}

func (r *PostgresTutorRepo) CreateTutor(ctx context.Context, t *domain.Tutor) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tutors (id, name, headline, bio, languages, avatar_url, hourly_gems, rating_avg, rating_count, user_id, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		t.ID, t.Name, t.Headline, t.Bio, strings.Join(t.Languages, ","), t.AvatarURL, t.HourlyGems, t.RatingAvg, t.RatingCount, t.UserID, t.Status)
	return err
}

func (r *PostgresTutorRepo) BookingsStarting(ctx context.Context, from, to time.Time) ([]domain.Booking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tutor_id, learner_id, slot_id, start_utc, duration_min, status, room_id, created_at, cost_gems
		   FROM tutor_bookings WHERE status=$1 AND start_utc >= $2 AND start_utc <= $3 ORDER BY start_utc`,
		domain.StatusConfirmed, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Booking{}
	for rows.Next() {
		var b domain.Booking
		if err := rows.Scan(&b.ID, &b.TutorID, &b.LearnerID, &b.SlotID, &b.StartUTC, &b.DurationMin, &b.Status, &b.RoomID, &b.CreatedAt, &b.CostGems); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresTutorRepo) SetStatus(ctx context.Context, tutorID, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE tutors SET status=$2 WHERE id=$1`, tutorID, status)
	return err
}

func (r *PostgresTutorRepo) AllBookings(ctx context.Context) ([]domain.Booking, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tutor_id, learner_id, slot_id, start_utc, duration_min, status, room_id, created_at, cost_gems
		   FROM tutor_bookings ORDER BY start_utc DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Booking{}
	for rows.Next() {
		var b domain.Booking
		if err := rows.Scan(&b.ID, &b.TutorID, &b.LearnerID, &b.SlotID, &b.StartUTC, &b.DurationMin, &b.Status, &b.RoomID, &b.CreatedAt, &b.CostGems); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PostgresTutorRepo) UpdateTutor(ctx context.Context, t *domain.Tutor) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tutors SET name=$2, headline=$3, bio=$4, languages=$5, avatar_url=$6, hourly_gems=$7, status=$8
		 WHERE id=$1`,
		t.ID, t.Name, t.Headline, t.Bio, strings.Join(t.Languages, ","), t.AvatarURL, t.HourlyGems, t.Status)
	return err
}

func (r *PostgresTutorRepo) DeleteTutor(ctx context.Context, tutorID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tutors WHERE id=$1`, tutorID)
	return err
}

func (r *PostgresTutorRepo) AddSlot(ctx context.Context, s *domain.Slot) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tutor_slots (id, tutor_id, start_utc, duration_min, booked)
		 VALUES ($1,$2,$3,$4,false)`,
		s.ID, s.TutorID, s.StartUTC, s.DurationMin)
	return err
}

func (r *PostgresTutorRepo) DeleteSlot(ctx context.Context, slotID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tutor_slots WHERE id = $1`, slotID)
	return err
}
