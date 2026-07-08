//go:build pgx

package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/classroom/domain"
)

type PostgresClassRepo struct{ pool *pgxpool.Pool }

func NewPostgresClassRepo(pool *pgxpool.Pool) *PostgresClassRepo {
	return &PostgresClassRepo{pool: pool}
}

const classCols = `id, tutor_id, title, description, start_utc, duration_min, capacity, room_id, created_at`

func scanClass(row interface{ Scan(dest ...any) error }) (*domain.ClassSession, error) {
	var c domain.ClassSession
	if err := row.Scan(&c.ID, &c.TutorID, &c.Title, &c.Description, &c.StartUTC, &c.DurationMin, &c.Capacity, &c.RoomID, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresClassRepo) CreateClass(ctx context.Context, c *domain.ClassSession) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO class_sessions (`+classCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		c.ID, c.TutorID, c.Title, c.Description, c.StartUTC, c.DurationMin, c.Capacity, c.RoomID, c.CreatedAt)
	return err
}

func (r *PostgresClassRepo) GetClass(ctx context.Context, id string) (*domain.ClassSession, error) {
	c, err := scanClass(r.pool.QueryRow(ctx, `SELECT `+classCols+` FROM class_sessions WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

func (r *PostgresClassRepo) query(ctx context.Context, sql string, args ...any) ([]domain.ClassSession, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ClassSession{}
	for rows.Next() {
		c, err := scanClass(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *PostgresClassRepo) ListUpcoming(ctx context.Context, now time.Time) ([]domain.ClassSession, error) {
	return r.query(ctx,
		`SELECT `+classCols+` FROM class_sessions
		   WHERE start_utc + (duration_min || ' minutes')::interval > $1 ORDER BY start_utc`, now)
}

func (r *PostgresClassRepo) ByTutor(ctx context.Context, tutorID string) ([]domain.ClassSession, error) {
	return r.query(ctx, `SELECT `+classCols+` FROM class_sessions WHERE tutor_id=$1 ORDER BY start_utc`, tutorID)
}

func (r *PostgresClassRepo) ClassesForStudent(ctx context.Context, studentID string) ([]domain.ClassSession, error) {
	return r.query(ctx,
		`SELECT `+classCols+` FROM class_sessions c
		   JOIN class_enrollments e ON e.class_id = c.id
		  WHERE e.student_id=$1 ORDER BY c.start_utc`, studentID)
}

func (r *PostgresClassRepo) Enroll(ctx context.Context, classID, studentID string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO class_enrollments (class_id, student_id, created_at) VALUES ($1,$2,now())
		 ON CONFLICT (class_id, student_id) DO NOTHING`, classID, studentID)
	return err
}

func (r *PostgresClassRepo) Unenroll(ctx context.Context, classID, studentID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM class_enrollments WHERE class_id=$1 AND student_id=$2`, classID, studentID)
	return err
}

func (r *PostgresClassRepo) IsEnrolled(ctx context.Context, classID, studentID string) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM class_enrollments WHERE class_id=$1 AND student_id=$2`, classID, studentID).Scan(&n)
	return n > 0, err
}

func (r *PostgresClassRepo) EnrollmentCount(ctx context.Context, classID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM class_enrollments WHERE class_id=$1`, classID).Scan(&n)
	return n, err
}

func (r *PostgresClassRepo) Roster(ctx context.Context, classID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT student_id FROM class_enrollments WHERE class_id=$1 ORDER BY student_id`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresClassRepo) ClassesStarting(ctx context.Context, from, to time.Time) ([]domain.ClassSession, error) {
	return r.query(ctx,
		`SELECT `+classCols+` FROM class_sessions WHERE start_utc >= $1 AND start_utc <= $2 ORDER BY start_utc`, from, to)
}

func (r *PostgresClassRepo) AllClasses(ctx context.Context) ([]domain.ClassSession, error) {
	return r.query(ctx, `SELECT `+classCols+` FROM class_sessions ORDER BY start_utc DESC`)
}
