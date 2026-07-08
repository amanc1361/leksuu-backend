//go:build pgx

package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/assignment/domain"
)

type PostgresAssignmentRepo struct{ pool *pgxpool.Pool }

func NewPostgresAssignmentRepo(pool *pgxpool.Pool) *PostgresAssignmentRepo {
	return &PostgresAssignmentRepo{pool: pool}
}

const assignCols = `id, tutor_id, student_id, type, title, prompt, lesson_id, due_utc,
	created_at, submission_text, submitted_at, score, feedback, graded_at`

func scanAssignment(row interface{ Scan(dest ...any) error }) (*domain.Assignment, error) {
	var a domain.Assignment
	if err := row.Scan(&a.ID, &a.TutorID, &a.StudentID, &a.Type, &a.Title, &a.Prompt, &a.LessonID,
		&a.DueUTC, &a.CreatedAt, &a.SubmissionText, &a.SubmittedAt, &a.Score, &a.Feedback, &a.GradedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *PostgresAssignmentRepo) Create(ctx context.Context, a *domain.Assignment) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO assignments (id, tutor_id, student_id, type, title, prompt, lesson_id, due_utc,
			created_at, submission_text, submitted_at, score, feedback, graded_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		a.ID, a.TutorID, a.StudentID, a.Type, a.Title, a.Prompt, a.LessonID, a.DueUTC,
		a.CreatedAt, a.SubmissionText, a.SubmittedAt, a.Score, a.Feedback, a.GradedAt)
	return err
}

func (r *PostgresAssignmentRepo) Get(ctx context.Context, id string) (*domain.Assignment, error) {
	a, err := scanAssignment(r.pool.QueryRow(ctx, `SELECT `+assignCols+` FROM assignments WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

func (r *PostgresAssignmentRepo) list(ctx context.Context, where string, arg string) ([]domain.Assignment, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+assignCols+` FROM assignments WHERE `+where+` ORDER BY created_at DESC`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Assignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *PostgresAssignmentRepo) ByTutor(ctx context.Context, tutorID string) ([]domain.Assignment, error) {
	return r.list(ctx, "tutor_id=$1", tutorID)
}

func (r *PostgresAssignmentRepo) ByStudent(ctx context.Context, studentID string) ([]domain.Assignment, error) {
	return r.list(ctx, "student_id=$1", studentID)
}

func (r *PostgresAssignmentRepo) Update(ctx context.Context, a *domain.Assignment) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE assignments SET submission_text=$2, submitted_at=$3, score=$4, feedback=$5, graded_at=$6 WHERE id=$1`,
		a.ID, a.SubmissionText, a.SubmittedAt, a.Score, a.Feedback, a.GradedAt)
	return err
}
