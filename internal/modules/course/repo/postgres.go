//go:build pgx

package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		`SELECT id, language, cefr_level, title, coalesce(description,''), coalesce(image_url,''), version, published, coalesce(source_id,'')
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
		if err := rows.Scan(&c.ID, &c.Language, &c.CEFR, &c.Title, &c.Description, &c.ImageURL, &c.Version, &c.Published, &c.SourceID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) Course(ctx context.Context, id string) (*domain.Course, error) {
	var c domain.Course
	err := r.pool.QueryRow(ctx,
		`SELECT id, language, cefr_level, title, coalesce(description,''), coalesce(image_url,''), version, published, coalesce(source_id,'')
		   FROM courses WHERE id=$1`, id).
		Scan(&c.ID, &c.Language, &c.CEFR, &c.Title, &c.Description, &c.ImageURL, &c.Version, &c.Published, &c.SourceID)
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
		`SELECT l.id, l.course_id, COALESCE(l.unit_id::text,''), l.position, l.title, l.version, COALESCE(l.cover_url,''), COALESCE(l.focus,''), COALESCE(l.objective,'{}'), COALESCE(l.source_id,''), COALESCE(l.kind,'lesson')
		   FROM lessons l LEFT JOIN units u ON u.id = l.unit_id
		   WHERE l.course_id=$1 ORDER BY COALESCE(u.position, 0), l.position`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Lesson
	for rows.Next() {
		var l domain.Lesson
		if err := rows.Scan(&l.ID, &l.CourseID, &l.UnitID, &l.Position, &l.Title, &l.Version, &l.CoverURL, &l.Focus, &l.Objective, &l.SourceID, &l.Kind); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) Lesson(ctx context.Context, id string) (*domain.Lesson, error) {
	var l domain.Lesson
	err := r.pool.QueryRow(ctx,
		`SELECT id, course_id, COALESCE(unit_id::text,''), position, title, version, COALESCE(cover_url,''), COALESCE(focus,''), COALESCE(objective,'{}'), COALESCE(source_id,''), COALESCE(kind,'lesson') FROM lessons WHERE id=$1`, id).
		Scan(&l.ID, &l.CourseID, &l.UnitID, &l.Position, &l.Title, &l.Version, &l.CoverURL, &l.Focus, &l.Objective, &l.SourceID, &l.Kind)
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
		`SELECT id, lesson_id, position, kind, ref_id, payload, media_key, COALESCE(source_ref,'')
		   FROM lesson_items WHERE lesson_id=$1 ORDER BY position`, lessonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LessonItem
	for rows.Next() {
		var it domain.LessonItem
		if err := rows.Scan(&it.ID, &it.LessonID, &it.Position, &it.Kind, &it.RefID, &it.Payload, &it.MediaKey, &it.SourceRef); err != nil {
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

// ExerciseLessonID returns the lesson an exercise belongs to.
func (r *PostgresCourseRepo) ExerciseLessonID(ctx context.Context, exerciseID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT li.lesson_id::text FROM exercises e JOIN lesson_items li ON li.id = e.lesson_item_id WHERE e.id=$1`,
		exerciseID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrExerciseNotFound
	}
	return id, err
}

// LessonItemCounts returns the number of items of every lesson in a course.
func (r *PostgresCourseRepo) LessonItemCounts(ctx context.Context, courseID string) (map[string]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT l.id::text, count(li.id)
		   FROM lessons l LEFT JOIN lesson_items li ON li.lesson_id = l.id
		  WHERE l.course_id=$1 GROUP BY l.id`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
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

// AttemptedExercises reports which of the given exercises the user has
// answered at least once.
// LatestExerciseScores returns the score of the user's most recent attempt at
// each given exercise (used to grade a checkpoint on its latest run).
func (r *PostgresProgressRepo) LatestExerciseScores(ctx context.Context, userID string, exerciseIDs []string) (map[string]float64, error) {
	out := map[string]float64{}
	if len(exerciseIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ON (entity_id) entity_id::text, COALESCE(score, 0)
		   FROM user_progress
		  WHERE user_id=$1 AND entity_type='exercise' AND entity_id::text = ANY($2::text[])
		  ORDER BY entity_id, occurred_at DESC`, userID, exerciseIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var score float64
		if err := rows.Scan(&id, &score); err != nil {
			return nil, err
		}
		out[id] = score
	}
	return out, rows.Err()
}

func (r *PostgresProgressRepo) AttemptedExercises(ctx context.Context, userID string, exerciseIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(exerciseIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT entity_id::text FROM user_progress
		  WHERE user_id=$1 AND entity_type='exercise' AND entity_id::text = ANY($2::text[])`, userID, exerciseIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
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
		`SELECT id, language, cefr_level, title, coalesce(description,''), coalesce(image_url,''), version, published, coalesce(source_id,'')
		   FROM courses ORDER BY language, cefr_level, title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Course
	for rows.Next() {
		var c domain.Course
		if err := rows.Scan(&c.ID, &c.Language, &c.CEFR, &c.Title, &c.Description, &c.ImageURL, &c.Version, &c.Published, &c.SourceID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) CreateCourse(ctx context.Context, c *domain.Course) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO courses (language, cefr_level, title, description, image_url, version, published, source_id)
		 VALUES ($1,$2,$3,$4,NULLIF($5,''), COALESCE(NULLIF($6,0),1), $7, NULLIF($8,'')) RETURNING id`,
		c.Language, c.CEFR, c.Title, c.Description, c.ImageURL, c.Version, c.Published, c.SourceID).Scan(&c.ID)
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
		   objective = COALESCE($5,'{}')::jsonb,
		   kind = COALESCE(NULLIF($6,''), kind)
		 WHERE id=$1 RETURNING id`,
		l.ID, l.Title, l.CoverURL, l.Focus, []byte(l.Objective), l.Kind).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLessonNotFound
	}
	return err
}

// MoveLessonToUnit moves a lesson to the end of another unit of the same course.
func (r *PostgresCourseRepo) MoveLessonToUnit(ctx context.Context, lessonID, unitID string) error {
	var id string
	err := r.pool.QueryRow(ctx,
		`UPDATE lessons SET
		   unit_id = $2,
		   position = (SELECT COALESCE(max(position),0)+1 FROM lessons WHERE unit_id=$2)
		 WHERE id=$1 AND course_id = (SELECT course_id FROM units WHERE id=$2)
		 RETURNING id`,
		lessonID, unitID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrLessonNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return domain.ErrLessonSourceConflict // lessons_unit_source_id_uq
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
	const where = `kind IN ('podcast','video','story') AND payload ? 'transcript'
		AND COALESCE((payload->>'published')::boolean, true)
		AND ($1='' OR kind=$1) AND ($2='' OR payload->>'language'=$2)`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM lesson_items WHERE `+where, kind, language).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, lesson_id, position, kind, ref_id, payload, media_key FROM lesson_items WHERE `+where+
			` ORDER BY COALESCE(NULLIF(payload->>'series_id',''), payload->>'title') NULLS LAST,
			CASE WHEN payload->>'part_number' ~ '^[0-9]+$' THEN (payload->>'part_number')::int ELSE 0 END,
			CASE WHEN payload->>'collection_position' ~ '^[0-9]+$' THEN (payload->>'collection_position')::int ELSE position END,
			id LIMIT $3 OFFSET $4`, kind, language, limit, offset)
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
		`INSERT INTO lesson_items (lesson_id, position, kind, ref_id, payload, media_key, source_ref)
		 VALUES ($1, COALESCE(NULLIF($2,0), (SELECT COALESCE(max(position),0)+1 FROM lesson_items WHERE lesson_id=$1)), $3, $4, COALESCE($5,'{}')::jsonb, $6, NULLIF($7,''))
		 RETURNING id, position`,
		it.LessonID, it.Position, it.Kind, it.RefID, []byte(it.Payload), it.MediaKey, it.SourceRef).Scan(&it.ID, &it.Position)
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

// ReplaceImportedItems syncs a lesson's imported items (source_ref set) to the
// given set in one transaction, so a failed re-import never leaves a
// half-filled lesson. Items are matched by (source_ref, occurrence): a matched
// item is updated in place and keeps its ID — and with it every learner row
// that references it (pronunciation scores, shadowing, exercise attempts) —
// new items are inserted and imported items no longer in the source are
// removed. Hand-authored items stay first; imported ones follow in the given
// order.
func (r *PostgresCourseRepo) ReplaceImportedItems(ctx context.Context, lessonID string, items []domain.ImportedItem) (*domain.ImportedItemsResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Serialize concurrent syncs of the same lesson.
	if _, err := tx.Exec(ctx, `SELECT id FROM lessons WHERE id=$1 FOR UPDATE`, lessonID); err != nil {
		return nil, err
	}

	type existingItem struct {
		id, kind string
		position int
		payload  []byte
	}
	rows, err := tx.Query(ctx,
		`SELECT id, kind, position, payload::text, source_ref FROM lesson_items
		  WHERE lesson_id=$1 AND source_ref IS NOT NULL ORDER BY position`, lessonID)
	if err != nil {
		return nil, err
	}
	byKey := map[string]existingItem{}
	seen := map[string]int{}
	var order []string
	for rows.Next() {
		var it existingItem
		var ref string
		if err := rows.Scan(&it.id, &it.kind, &it.position, &it.payload, &ref); err != nil {
			rows.Close()
			return nil, err
		}
		key := importKey(ref, seen)
		byKey[key] = it
		order = append(order, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(byKey))
	for _, it := range byKey {
		ids = append(ids, it.id)
	}
	exByItem := map[string][]domain.Exercise{}
	if len(ids) > 0 {
		erows, err := tx.Query(ctx,
			`SELECT id, lesson_item_id, type, prompt::text, answer_key::text, difficulty FROM exercises
			  WHERE lesson_item_id::text = ANY($1::text[]) ORDER BY lesson_item_id, id`, ids)
		if err != nil {
			return nil, err
		}
		for erows.Next() {
			var e domain.Exercise
			var prompt, key []byte
			if err := erows.Scan(&e.ID, &e.LessonItemID, &e.Type, &prompt, &key, &e.Difficulty); err != nil {
				erows.Close()
				return nil, err
			}
			e.Prompt, e.AnswerKey = prompt, key
			exByItem[e.LessonItemID] = append(exByItem[e.LessonItemID], e)
		}
		erows.Close()
		if err := erows.Err(); err != nil {
			return nil, err
		}
	}

	// Imported items move out of the way of the (lesson_id, position) unique
	// key; hand-authored ones keep their positions and stay first.
	if _, err := tx.Exec(ctx, `UPDATE lesson_items SET position = position + 100000 WHERE lesson_id=$1 AND source_ref IS NOT NULL`, lessonID); err != nil {
		return nil, err
	}
	var base int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(position),0) FROM lesson_items WHERE lesson_id=$1 AND source_ref IS NULL`, lessonID).Scan(&base); err != nil {
		return nil, err
	}

	res := &domain.ImportedItemsResult{Items: make([]domain.LessonItem, len(items))}
	matched := map[string]bool{}
	seen = map[string]int{}
	batch := &pgx.Batch{}
	type pending struct{ index int }
	var inserts []pending
	for i, in := range items {
		pos := base + i + 1
		payload := in.Payload
		if len(payload) == 0 {
			payload = []byte(`{}`)
		}
		out := domain.LessonItem{LessonID: lessonID, Position: pos, Kind: in.Kind, Payload: payload, SourceRef: in.SourceRef}
		key := importKey(in.SourceRef, seen)
		old, ok := byKey[key]
		if !ok {
			inserts = append(inserts, pending{i})
			res.Items[i] = out
			continue
		}
		matched[key] = true
		out.ID = old.id
		res.Items[i] = out
		changed := old.kind != in.Kind || old.position != pos || !sameJSON(old.payload, payload)
		batch.Queue(`UPDATE lesson_items SET kind=$2, payload=COALESCE($3,'{}')::jsonb, position=$4 WHERE id=$1`,
			old.id, in.Kind, []byte(payload), pos)
		olds := exByItem[old.id]
		for j, e := range in.Exercises {
			if j < len(olds) {
				o := olds[j]
				if o.Type != e.Type || o.Difficulty != e.Difficulty || !sameJSON(o.Prompt, e.Prompt) || !sameJSON(o.AnswerKey, e.AnswerKey) {
					changed = true
					batch.Queue(`UPDATE exercises SET type=$2, prompt=$3, answer_key=$4, difficulty=$5 WHERE id=$1`,
						o.ID, e.Type, []byte(e.Prompt), []byte(e.AnswerKey), e.Difficulty)
				}
				continue
			}
			changed = true
			batch.Queue(`INSERT INTO exercises (lesson_item_id, type, prompt, answer_key, difficulty) VALUES ($1,$2,$3,$4,$5)`,
				old.id, e.Type, []byte(e.Prompt), []byte(e.AnswerKey), e.Difficulty)
		}
		for _, o := range olds[min(len(in.Exercises), len(olds)):] {
			changed = true
			batch.Queue(`DELETE FROM exercises WHERE id=$1`, o.ID)
		}
		if changed {
			res.Updated++
		} else {
			res.Unchanged++
		}
	}
	for _, key := range order {
		if !matched[key] {
			batch.Queue(`DELETE FROM lesson_items WHERE id=$1`, byKey[key].id)
			res.Removed++
		}
	}
	if batch.Len() > 0 {
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return nil, err
		}
	}

	// New items: one round trip for the items, one for their exercises.
	if len(inserts) > 0 {
		ib := &pgx.Batch{}
		for _, p := range inserts {
			it := res.Items[p.index]
			ib.Queue(`INSERT INTO lesson_items (lesson_id, position, kind, payload, source_ref)
			          VALUES ($1, $2, $3, COALESCE($4,'{}')::jsonb, $5) RETURNING id`,
				lessonID, it.Position, it.Kind, []byte(it.Payload), it.SourceRef)
		}
		br := tx.SendBatch(ctx, ib)
		for _, p := range inserts {
			if err := br.QueryRow().Scan(&res.Items[p.index].ID); err != nil {
				br.Close()
				return nil, err
			}
		}
		if err := br.Close(); err != nil {
			return nil, err
		}
		eb := &pgx.Batch{}
		for _, p := range inserts {
			for _, e := range items[p.index].Exercises {
				eb.Queue(`INSERT INTO exercises (lesson_item_id, type, prompt, answer_key, difficulty) VALUES ($1,$2,$3,$4,$5)`,
					res.Items[p.index].ID, e.Type, []byte(e.Prompt), []byte(e.AnswerKey), e.Difficulty)
			}
		}
		if eb.Len() > 0 {
			if err := tx.SendBatch(ctx, eb).Close(); err != nil {
				return nil, err
			}
		}
		res.Created = len(inserts)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
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
		`SELECT id, course_id, position, title, COALESCE(description,''), COALESCE(source_id,'')
		   FROM units WHERE course_id=$1 ORDER BY position`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Unit
	for rows.Next() {
		var u domain.Unit
		if err := rows.Scan(&u.ID, &u.CourseID, &u.Position, &u.Title, &u.Description, &u.SourceID); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *PostgresCourseRepo) Unit(ctx context.Context, unitID string) (*domain.Unit, error) {
	var u domain.Unit
	err := r.pool.QueryRow(ctx,
		`SELECT id, course_id, position, title, COALESCE(description,''), COALESCE(source_id,'') FROM units WHERE id=$1`, unitID).
		Scan(&u.ID, &u.CourseID, &u.Position, &u.Title, &u.Description, &u.SourceID)
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
		`INSERT INTO units (course_id, position, title, description, source_id)
		 VALUES ($1, COALESCE(NULLIF($2,0), (SELECT COALESCE(max(position),0)+1 FROM units WHERE course_id=$1)), $3, NULLIF($4,''), NULLIF($5,''))
		 RETURNING id, position`,
		u.CourseID, u.Position, u.Title, u.Description, u.SourceID).Scan(&u.ID, &u.Position)
}

// SetUnitSourceID records the external chapter a unit was imported from (used
// when a re-import adopts a unit an older importer created by title).
func (r *PostgresCourseRepo) SetUnitSourceID(ctx context.Context, unitID, sourceID string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE units SET source_id = NULLIF($2,'') WHERE id=$1`, unitID, sourceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUnitNotFound
	}
	return nil
}

// LessonsBySourceID finds a course's lessons imported from one external lesson
// (normally zero or one; more only if an older importer duplicated it).
func (r *PostgresCourseRepo) LessonsBySourceID(ctx context.Context, courseID, sourceID string) ([]domain.Lesson, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT l.id, l.course_id, COALESCE(l.unit_id::text,''), l.position, l.title, l.version, COALESCE(l.cover_url,''), COALESCE(l.focus,''), COALESCE(l.objective,'{}'), COALESCE(l.source_id,'')
		   FROM lessons l LEFT JOIN units u ON u.id = l.unit_id
		  WHERE l.course_id=$1 AND l.source_id=$2 ORDER BY COALESCE(u.position, 0), l.position`, courseID, sourceID)
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

func (r *PostgresCourseRepo) LessonsByUnit(ctx context.Context, unitID string) ([]domain.Lesson, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, course_id, COALESCE(unit_id::text,''), position, title, version, COALESCE(cover_url,''), COALESCE(focus,''), COALESCE(objective,'{}'), COALESCE(source_id,''), COALESCE(kind,'lesson')
		   FROM lessons WHERE unit_id=$1 ORDER BY position`, unitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Lesson
	for rows.Next() {
		var l domain.Lesson
		if err := rows.Scan(&l.ID, &l.CourseID, &l.UnitID, &l.Position, &l.Title, &l.Version, &l.CoverURL, &l.Focus, &l.Objective, &l.SourceID, &l.Kind); err != nil {
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
		`SELECT id, language, cefr_level, title, coalesce(description,''), coalesce(image_url,''), version, published, coalesce(source_id,'')
		   FROM courses WHERE ($1='' OR title ILIKE '%'||$1||'%')
		   ORDER BY language, cefr_level, title LIMIT $2 OFFSET $3`, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Course
	for rows.Next() {
		var c domain.Course
		if err := rows.Scan(&c.ID, &c.Language, &c.CEFR, &c.Title, &c.Description, &c.ImageURL, &c.Version, &c.Published, &c.SourceID); err != nil {
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

// --- localized content ---

func (r *PostgresCourseRepo) ContentTranslations(ctx context.Context, hashes []string) ([]domain.ContentTranslation, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT source_hash, locale, source_lang, source_text, text, origin
		   FROM content_translations WHERE source_hash = ANY($1::text[]) ORDER BY source_hash, locale`, hashes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ContentTranslation
	for rows.Next() {
		var t domain.ContentTranslation
		if err := rows.Scan(&t.SourceHash, &t.Locale, &t.SourceLang, &t.SourceText, &t.Text, &t.Origin); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpsertContentTranslations writes translations in one transaction.
func (r *PostgresCourseRepo) UpsertContentTranslations(ctx context.Context, ts []domain.ContentTranslation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	b := &pgx.Batch{}
	for _, t := range ts {
		b.Queue(`INSERT INTO content_translations (source_hash, locale, source_lang, source_text, text, origin, updated_at)
		         VALUES ($1,$2,$3,$4,$5,$6, now())
		         ON CONFLICT (source_hash, locale) DO UPDATE
		            SET text=EXCLUDED.text, origin=EXCLUDED.origin, source_lang=EXCLUDED.source_lang,
		                source_text=EXCLUDED.source_text, updated_at=now()`,
			t.SourceHash, t.Locale, t.SourceLang, t.SourceText, t.Text, t.Origin)
	}
	if err := tx.SendBatch(ctx, b).Close(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresCourseRepo) DeleteContentTranslation(ctx context.Context, sourceHash, locale string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM content_translations WHERE source_hash=$1 AND locale=$2`, sourceHash, locale)
	return err
}

// Languages returns every target language, ordered by position then code.
func (r *PostgresCourseRepo) Languages(ctx context.Context) ([]domain.Language, error) {
	rows, err := r.pool.Query(ctx, `SELECT code, enabled, position FROM languages ORDER BY position, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Language
	for rows.Next() {
		var l domain.Language
		if err := rows.Scan(&l.Code, &l.Enabled, &l.Position); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SaveLanguage inserts or updates a language.
func (r *PostgresCourseRepo) SaveLanguage(ctx context.Context, l domain.Language) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO languages (code, enabled, position) VALUES ($1, $2, $3)
		 ON CONFLICT (code) DO UPDATE SET enabled = EXCLUDED.enabled, position = EXCLUDED.position`,
		l.Code, l.Enabled, l.Position)
	return err
}
