//go:build pgx

package repo

import (
	"context"

	"github.com/leksuu/platform/internal/modules/course/domain"
)

func (r *PostgresCourseRepo) SaveImportSource(ctx context.Context, src *domain.ImportSource) error {
	meta := []byte(src.Metadata)
	if len(meta) == 0 {
		meta = []byte("{}")
	}
	return r.pool.QueryRow(ctx,
		`INSERT INTO course_import_sources
		   (file_name, storage_key, url, content_type, content_bytes, content_sha256, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,COALESCE($7,'{}')::jsonb)
		 ON CONFLICT (storage_key) DO UPDATE SET
		   file_name = EXCLUDED.file_name,
		   url = EXCLUDED.url,
		   content_type = EXCLUDED.content_type,
		   content_bytes = EXCLUDED.content_bytes,
		   content_sha256 = EXCLUDED.content_sha256,
		   metadata = EXCLUDED.metadata
		 RETURNING id, created_at`,
		src.FileName, src.StorageKey, src.URL, src.ContentType, src.ContentBytes, src.SHA256, meta).
		Scan(&src.ID, &src.CreatedAt)
}

func (r *PostgresCourseRepo) ListImportSources(ctx context.Context, limit, offset int) ([]domain.ImportSource, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM course_import_sources`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, file_name, storage_key, url, content_type, content_bytes, content_sha256, metadata, created_at
		   FROM course_import_sources
		  ORDER BY created_at DESC
		  LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]domain.ImportSource, 0)
	for rows.Next() {
		var src domain.ImportSource
		if err := rows.Scan(&src.ID, &src.FileName, &src.StorageKey, &src.URL, &src.ContentType, &src.ContentBytes, &src.SHA256, &src.Metadata, &src.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, src)
	}
	return out, total, rows.Err()
}
