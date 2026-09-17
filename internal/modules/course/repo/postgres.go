//go:build pgx

package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/course/domain"
)

// --- courses / lessons / items / exercises ---

type PostgresCourseRepo struct{ pool *pgxpool.Pool }

func NewPostgresCourseRepo(pool *pgxpool.Pool) *PostgresCourseRepo {
	return &PostgresCourseRepo{pool: pool}
}

func (r *PostgresCourseRepo) ListCourses(ctx context.Context, language, cefr string) ([]domain.Course, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, language, cefr_level, title, coalesce(description,''), coalesce(image_url,''), version, published
		   FROM courses
		  WHERE published
		    AND ($1 = '' OR language = $1)
		    AND ($2 = '' OR cefr_level = $2)
		  ORDER BY language, cefr_level, title`, language, cefr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Course
	for rows.Next() {
		var c domain.Course
		if err := rows.Scan(&c.ID, &c.Language, &c.CEFR, &c.Title, &c.Description, &c.ImageURL, &c.Version, &c.Published); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) Course(ctx context.Context, id string) (*domain.Course, error) {
	var c domain.Course
	err := r.pool.QueryRow(ctx,
		`SELECT id, language, cefr_level, title, coalesce(description,''), coalesce(image_url,''), version, published
		   FROM courses WHERE id=$1`, id).
		Scan(&c.ID, &c.Language, &c.CEFR, &c.Title, &c.Description, &c.ImageURL, &c.Version, &c.Published)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrCourseNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresCourseRepo) Lessons(ctx context.Context, courseID string) ([]domain.Lesson, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT l.id, l.course_id, COALESCE(l.unit_id::text,''), l.position, l.title, l.version, COALESCE(l.cover_url,''), COALESCE(l.focus,''), COALESCE(l.objective,'{}'), COALESCE(l.source_id,'')
		   FROM lessons l LEFT JOIN units u ON u.id = l.unit_id
		   WHERE l.course_id=$1 ORDER BY COALESCE(u.position, 0), l.position`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Lesson
	for rows.Next() {
		var l domain.Lesson
		if err := rows.Scan(&l.ID, &l.CourseID, &l.UnitID, &l.Position, &l.Title, &l.Version, &l.CoverURL, &l.Focus, &l.Objective, &l.SourceID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) Lesson(ctx context.Context, id string) (*domain.Lesson, error) {
	var l domain.Lesson
	err := r.pool.QueryRow(ctx,
		`SELECT id, course_id, COALESCE(unit_id::text,''), position, title, version, COALESCE(cover_url,''), COALESCE(focus,''), COALESCE(objective,'{}'), COALESCE(source_id,'') FROM lessons WHERE id=$1`, id).
		Scan(&l.ID, &l.CourseID, &l.UnitID, &l.Position, &l.Title, &l.Version, &l.CoverURL, &l.Focus, &l.Objective, &l.SourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrLessonNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *PostgresCourseRepo) Items(ctx context.Context, lessonID string) ([]domain.LessonItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, lesson_id, position, kind, ref_id, payload, media_key
		   FROM lesson_items WHERE lesson_id=$1 ORDER BY position`, lessonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LessonItem
	for rows.Next() {
		var it domain.LessonItem
		if err := rows.Scan(&it.ID, &it.LessonID, &it.Position, &it.Kind, &it.RefID, &it.Payload, &it.MediaKey); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) Exercises(ctx context.Context, itemID string) ([]domain.Exercise, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, lesson_item_id, type, prompt, answer_key, difficulty
		   FROM exercises WHERE lesson_item_id=$1 ORDER BY id`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Exercise
	for rows.Next() {
		var e domain.Exercise
		if err := rows.Scan(&e.ID, &e.LessonItemID, &e.Type, &e.Prompt, &e.AnswerKey, &e.Difficulty); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) Exercise(ctx context.Context, id string) (*domain.Exercise, error) {
	var e domain.Exercise
	err := r.pool.QueryRow(ctx,
		`SELECT id, lesson_item_id, type, prompt, answer_key, difficulty FROM exercises WHERE id=$1`, id).
		Scan(&e.ID, &e.LessonItemID, &e.Type, &e.Prompt, &e.AnswerKey, &e.Difficulty)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrExerciseNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *PostgresCourseRepo) LessonCount(ctx context.Context, courseID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM lessons WHERE course_id=$1`, courseID).Scan(&n)
	return n, err
}

// --- progress ---

type PostgresProgressRepo struct{ pool *pgxpool.Pool }

func NewPostgresProgressRepo(pool *pgxpool.Pool) *PostgresProgressRepo {
	return &PostgresProgressRepo{pool: pool}
}

func (r *PostgresProgressRepo) Record(ctx context.Context, e domain.ProgressEntry) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_progress (user_id, entity_type, entity_id, status, score, occurred_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		e.UserID, e.EntityType, e.EntityID, e.Status, e.Score, e.OccurredAt)
	return err
}

func (r *PostgresProgressRepo) ExerciseAttempts(ctx context.Context, userID, exerciseID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM user_progress
		  WHERE user_id=$1 AND entity_type='exercise' AND entity_id=$2`, userID, exerciseID).Scan(&n)
	return n, err
}

func (r *PostgresProgressRepo) CompletedLessons(ctx context.Context, userID, courseID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(DISTINCT up.entity_id)
		   FROM user_progress up
		   JOIN lessons l ON l.id = up.entity_id
		  WHERE up.user_id=$1 AND up.entity_type='lesson' AND up.status='completed'
		    AND l.course_id=$2`, userID, courseID).Scan(&n)
	return n, err
}

func (r *PostgresProgressRepo) CompletedLessonIDs(ctx context.Context, userID, courseID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT up.entity_id
		   FROM user_progress up
		   JOIN lessons l ON l.id = up.entity_id
		  WHERE up.user_id=$1 AND up.entity_type='lesson' AND up.status='completed'
		    AND l.course_id=$2`, userID, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ---- write side (admin module) ----

func (r *PostgresCourseRepo) ListAllCourses(ctx context.Context) ([]domain.Course, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, language, cefr_level, title, coalesce(description,''), coalesce(image_url,''), version, published
		   FROM courses ORDER BY language, cefr_level, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Course
	for rows.Next() {
		var c domain.Course
		if err := rows.Scan(&c.ID, &c.Language, &c.CEFR, &c.Title, &c.Description, &c.ImageURL, &c.Version, &c.Published); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) CreateCourse(ctx context.Context, c *domain.Course) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO courses (language, cefr_level, title, description, image_url, version, published)
		 VALUES ($1,$2,$3,$4,NULLIF($5,''), COALESCE(NULLIF($6,0),1), $7) RETURNING id`,
		c.Language, c.CEFR, c.Title, c.Description, c.ImageURL, c.Version, c.Published).Scan(&c.ID)
}

func (r *PostgresCourseRepo) SetPublished(ctx context.Context, courseID string, published bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE courses SET published=$2, updated_at=now() WHERE id=$1`, courseID, published)
	return err
}

func (r *PostgresCourseRepo) UpdateLesson(ctx context.Context, l *domain.Lesson) error {
	var id string
	err := r.pool.QueryRow(ctx,
		`UPDATE lessons SET
		   title = COALESCE(NULLIF($2,''), title),
		   cover_url = NULLIF($3,''),
		   focus = NULLIF($4,''),
		   objective = COALESCE($5,'{}')::jsonb
		 WHERE id=$1 RETURNING id`,
		l.ID, l.Title, l.CoverURL, l.Focus, []byte(l.Objective)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLessonNotFound
	}
	return err
}

func (r *PostgresCourseRepo) CreateLesson(ctx context.Context, l *domain.Lesson) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO lessons (course_id, unit_id, position, title, version, source_id)
		 VALUES (
		   (SELECT course_id FROM units WHERE id=$1),
		   $1,
		   COALESCE(NULLIF($2,0), (SELECT COALESCE(max(position),0)+1 FROM lessons WHERE unit_id=$1)),
		   $3, COALESCE(NULLIF($4,0),1), NULLIF($5,''))
		 RETURNING id, course_id, position`,
		l.UnitID, l.Position, l.Title, l.Version, l.SourceID).Scan(&l.ID, &l.CourseID, &l.Position)
}

func (r *PostgresCourseRepo) MediaItems(ctx context.Context, kind, language string, limit, offset int) ([]domain.LessonItem, int, error) {
	if limit <= 0 {
		limit = 50
	}
	const where = `kind IN ('podcast','video','story') AND payload ? 'transcript' AND ($1='' OR kind=$1) AND ($2='' OR payload->>'language'=$2)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM lesson_items WHERE `+where, kind, language).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, lesson_id, position, kind, ref_id, payload, media_key FROM lesson_items WHERE `+where+
			` ORDER BY payload->>'title' NULLS LAST, id LIMIT $3 OFFSET $4`, kind, language, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.LessonItem
	for rows.Next() {
		var it domain.LessonItem
		if err := rows.Scan(&it.ID, &it.LessonID, &it.Position, &it.Kind, &it.RefID, &it.Payload, &it.MediaKey); err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

func (r *PostgresCourseRepo) UpdateItem(ctx context.Context, it *domain.LessonItem) error {
	var id string
	err := r.pool.QueryRow(ctx,
		`UPDATE lesson_items SET payload = COALESCE($2,'{}')::jsonb, media_key = $3 WHERE id = $1 RETURNING id`,
		it.ID, []byte(it.Payload), it.MediaKey).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrItemNotFound
	}
	return err
}

func (r *PostgresCourseRepo) CreateItem(ctx context.Context, it *domain.LessonItem) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO lesson_items (lesson_id, position, kind, ref_id, payload, media_key)
		 VALUES ($1, COALESCE(NULLIF($2,0), (SELECT COALESCE(max(position),0)+1 FROM lesson_items WHERE lesson_id=$1)), $3, $4, COALESCE($5,'{}')::jsonb, $6)
		 RETURNING id, position`,
		it.LessonID, it.Position, it.Kind, it.RefID, []byte(it.Payload), it.MediaKey).Scan(&it.ID, &it.Position)
}

func (r *PostgresCourseRepo) CreateExercise(ctx context.Context, e *domain.Exercise) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO exercises (lesson_item_id, type, prompt, answer_key, difficulty)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		e.LessonItemID, e.Type, []byte(e.Prompt), []byte(e.AnswerKey), e.Difficulty).Scan(&e.ID)
}

func (r *PostgresCourseRepo) DeleteItem(ctx context.Context, itemID string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM exercises WHERE lesson_item_id=$1`, itemID); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM lesson_items WHERE id=$1`, itemID)
	return err
}

func (r *PostgresCourseRepo) ReorderItems(ctx context.Context, lessonID string, orderedIDs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// move all out of the unique range first to avoid (lesson_id, position) clashes
	if _, err := tx.Exec(ctx, `UPDATE lesson_items SET position = position + 100000 WHERE lesson_id=$1`, lessonID); err != nil {
		return err
	}
	for i, id := range orderedIDs {
		if _, err := tx.Exec(ctx, `UPDATE lesson_items SET position=$1 WHERE id=$2 AND lesson_id=$3`, i+1, id, lessonID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// --- units (chapters) ---

func (r *PostgresCourseRepo) Units(ctx context.Context, courseID string) ([]domain.Unit, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, course_id, position, title, COALESCE(description,'')
		   FROM units WHERE course_id=$1 ORDER BY position`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Unit
	for rows.Next() {
		var u domain.Unit
		if err := rows.Scan(&u.ID, &u.CourseID, &u.Position, &u.Title, &u.Description); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) Unit(ctx context.Context, unitID string) (*domain.Unit, error) {
	var u domain.Unit
	err := r.pool.QueryRow(ctx,
		`SELECT id, course_id, position, title, COALESCE(description,'') FROM units WHERE id=$1`, unitID).
		Scan(&u.ID, &u.CourseID, &u.Position, &u.Title, &u.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnitNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *PostgresCourseRepo) CreateUnit(ctx context.Context, u *domain.Unit) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO units (course_id, position, title, description)
		 VALUES ($1, COALESCE(NULLIF($2,0), (SELECT COALESCE(max(position),0)+1 FROM units WHERE course_id=$1)), $3, NULLIF($4,''))
		 RETURNING id, position`,
		u.CourseID, u.Position, u.Title, u.Description).Scan(&u.ID, &u.Position)
}

func (r *PostgresCourseRepo) LessonsByUnit(ctx context.Context, unitID string) ([]domain.Lesson, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, course_id, COALESCE(unit_id::text,''), position, title, version, COALESCE(cover_url,''), COALESCE(focus,''), COALESCE(objective,'{}'), COALESCE(source_id,'')
		   FROM lessons WHERE unit_id=$1 ORDER BY position`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Lesson
	for rows.Next() {
		var l domain.Lesson
		if err := rows.Scan(&l.ID, &l.CourseID, &l.UnitID, &l.Position, &l.Title, &l.Version, &l.CoverURL, &l.Focus, &l.Objective, &l.SourceID); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) ReorderUnits(ctx context.Context, courseID string, orderedIDs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE units SET position = position + 100000 WHERE course_id=$1`, courseID); err != nil {
		return err
	}
	for i, id := range orderedIDs {
		if _, err := tx.Exec(ctx, `UPDATE units SET position=$1 WHERE id=$2 AND course_id=$3`, i+1, id, courseID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresCourseRepo) ReorderLessons(ctx context.Context, unitID string, orderedIDs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE lessons SET position = position + 100000 WHERE unit_id=$1`, unitID); err != nil {
		return err
	}
	for i, id := range orderedIDs {
		if _, err := tx.Exec(ctx, `UPDATE lessons SET position=$1 WHERE id=$2 AND unit_id=$3`, i+1, id, unitID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *PostgresCourseRepo) SearchCourses(ctx context.Context, q string, limit, offset int) ([]domain.Course, int, error) {
	if limit <= 0 {
		limit = 1000
	}
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM courses WHERE ($1='' OR title ILIKE '%'||$1||'%')`, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, language, cefr_level, title, coalesce(description,''), coalesce(image_url,''), version, published
		   FROM courses WHERE ($1='' OR title ILIKE '%'||$1||'%')
		   ORDER BY language, cefr_level, title LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Course
	for rows.Next() {
		var c domain.Course
		if err := rows.Scan(&c.ID, &c.Language, &c.CEFR, &c.Title, &c.Description, &c.ImageURL, &c.Version, &c.Published); err != nil {
			return nil, 0, err
		}
		out = append(out, c)
	}
	return out, total, rows.Err()
}

func (r *PostgresCourseRepo) GetAccessRule(ctx context.Context, entityType, entityID string) (*domain.AccessRule, error) {
	var ar domain.AccessRule
	err := r.pool.QueryRow(ctx,
		`SELECT entity_type, entity_id, requirement FROM access_rules WHERE entity_type=$1 AND entity_id=$2`,
		entityType, entityID).Scan(&ar.EntityType, &ar.EntityID, &ar.Requirement)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ar, nil
}

func (r *PostgresCourseRepo) SetAccessRule(ctx context.Context, rule domain.AccessRule) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO access_rules (entity_type, entity_id, requirement) VALUES ($1,$2,$3)
		 ON CONFLICT (entity_type, entity_id) DO UPDATE SET requirement=$3`,
		rule.EntityType, rule.EntityID, rule.Requirement)
	return err
}

func (r *PostgresCourseRepo) DeleteAccessRule(ctx context.Context, entityType, entityID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM access_rules WHERE entity_type=$1 AND entity_id=$2`, entityType, entityID)
	return err
}

func (r *PostgresCourseRepo) ListAccessRules(ctx context.Context) ([]domain.AccessRule, error) {
	rows, err := r.pool.Query(ctx, `SELECT entity_type, entity_id, requirement FROM access_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AccessRule{}
	for rows.Next() {
		var ar domain.AccessRule
		if err := rows.Scan(&ar.EntityType, &ar.EntityID, &ar.Requirement); err != nil {
			return nil, err
		}
		out = append(out, ar)
	}
	return out, rows.Err()
}
