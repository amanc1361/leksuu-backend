//go:build pgx

package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/recording/domain"
)

// PostgresRepo stores recording metadata.
type PostgresRepo struct{ pool *pgxpool.Pool }

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo { return &PostgresRepo{pool: pool} }

func (r *PostgresRepo) Create(ctx context.Context, rec *domain.Recording) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO recordings (id, class_id, host_id, status, size_bytes, url, storage_key, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		rec.ID, rec.ClassID, rec.HostID, rec.Status, rec.SizeBytes, rec.URL, rec.Key, rec.CreatedAt)
	return err
}

func (r *PostgresRepo) Get(ctx context.Context, id string) (*domain.Recording, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, class_id, host_id, status, size_bytes, url, storage_key, created_at, ready_at
		 FROM recordings WHERE id=$1`, id)
	var rec domain.Recording
	if err := row.Scan(&rec.ID, &rec.ClassID, &rec.HostID, &rec.Status, &rec.SizeBytes, &rec.URL, &rec.Key, &rec.CreatedAt, &rec.ReadyAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

func (r *PostgresRepo) Update(ctx context.Context, rec *domain.Recording) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE recordings SET status=$2, size_bytes=$3, url=$4, storage_key=$5, ready_at=$6 WHERE id=$1`,
		rec.ID, rec.Status, rec.SizeBytes, rec.URL, rec.Key, rec.ReadyAt)
	return err
}

func (r *PostgresRepo) ListByClass(ctx context.Context, classID string) ([]domain.Recording, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, class_id, host_id, status, size_bytes, url, storage_key, created_at, ready_at
		 FROM recordings WHERE class_id=$1 ORDER BY created_at DESC`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Recording
	for rows.Next() {
		var rec domain.Recording
		if err := rows.Scan(&rec.ID, &rec.ClassID, &rec.HostID, &rec.Status, &rec.SizeBytes, &rec.URL, &rec.Key, &rec.CreatedAt, &rec.ReadyAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *PostgresRepo) ListAll(ctx context.Context) ([]domain.Recording, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, class_id, host_id, status, size_bytes, url, storage_key, created_at, ready_at
		 FROM recordings ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Recording
	for rows.Next() {
		var rec domain.Recording
		if err := rows.Scan(&rec.ID, &rec.ClassID, &rec.HostID, &rec.Status, &rec.SizeBytes, &rec.URL, &rec.Key, &rec.CreatedAt, &rec.ReadyAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *PostgresRepo) Delete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM recordings WHERE id=$1`, id)
	return err
}
