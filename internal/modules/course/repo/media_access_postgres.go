//go:build pgx

package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/leksuu/platform/internal/modules/course/domain"
)

// PlaybackItem uses the same authoritative lesson record as the library.
func (r *PostgresCourseRepo) PlaybackItem(ctx context.Context, id string) (*domain.LessonItem, error) {
	var it domain.LessonItem
	err := r.pool.QueryRow(ctx, `SELECT id,lesson_id,position,kind,ref_id,payload,media_key
		FROM lesson_items WHERE id=$1 AND kind IN ('video','music')`, id).
		Scan(&it.ID, &it.LessonID, &it.Position, &it.Kind, &it.RefID, &it.Payload, &it.MediaKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	return &it, nil
}

func (r *PostgresCourseRepo) MediaLibraryBucket(ctx context.Context, id string) (bool, error) {
	var bucket bool
	err := r.pool.QueryRow(ctx, `SELECT c.title='__media_library__' FROM lesson_items li
		JOIN lessons l ON l.id=li.lesson_id JOIN courses c ON c.id=l.course_id WHERE li.id=$1`, id).Scan(&bucket)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrItemNotFound
	}
	return bucket, err
}

// ProtectedMediaKey also protects legacy Film/Music objects with random keys.
// All HLS/large keys are blocked independently by the byte router.
func (r *PostgresCourseRepo) ProtectedMediaKey(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lesson_items
		WHERE kind IN ('video','music') AND (
			media_key=$1 OR split_part(payload->>'url','?',1) LIKE '%/media/' || $1
			OR payload->>'hls_manifest_key'=$1))`, key).Scan(&exists)
	return exists, err
}
