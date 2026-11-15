//go:build pgx

package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
)

type PostgresMediaJobRepo struct{ pool *pgxpool.Pool }

func NewPostgresMediaJobRepo(pool *pgxpool.Pool) *PostgresMediaJobRepo {
	return &PostgresMediaJobRepo{pool: pool}
}

const jobColumns = `id::text,item_id::text,kind,operation,target_language,payload_sha256,status,progress,attempts,cancel_requested,result,error_code,events,created_at,updated_at`

func scanMediaJob(row pgx.Row) (*coursedomain.MediaProcessingJob, error) {
	var j coursedomain.MediaProcessingJob
	var result, events []byte
	var errorCode *string
	err := row.Scan(&j.ID, &j.ItemID, &j.Kind, &j.Operation, &j.TargetLanguage, &j.PayloadSHA256, &j.Status, &j.Progress, &j.Attempts, &j.CancelRequested, &result, &errorCode, &events, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, coursedomain.ErrItemNotFound
	}
	if err != nil {
		return nil, err
	}
	j.Result = json.RawMessage(result)
	j.Events = json.RawMessage(events)
	if errorCode != nil {
		j.ErrorCode = *errorCode
	}
	return &j, nil
}

func (r *PostgresMediaJobRepo) Enqueue(ctx context.Context, itemID, kind, operation, targetLanguage, payloadSHA256 string, payload json.RawMessage) (*coursedomain.MediaProcessingJob, error) {
	return scanMediaJob(r.pool.QueryRow(ctx, `INSERT INTO media_processing_jobs(item_id,kind,operation,target_language,payload_sha256,payload_snapshot,events)
		VALUES($1::uuid,$2,$3,$4,$5,$6::jsonb,'[{"event":"queued"}]'::jsonb)
		ON CONFLICT(item_id,operation,target_language,payload_sha256) WHERE status IN ('pending','running','review','ready')
		DO UPDATE SET updated_at=media_processing_jobs.updated_at RETURNING `+jobColumns, itemID, kind, operation, targetLanguage, payloadSHA256, payload))
}

func (r *PostgresMediaJobRepo) List(ctx context.Context, limit int) ([]coursedomain.MediaProcessingJob, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+jobColumns+` FROM media_processing_jobs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]coursedomain.MediaProcessingJob, 0)
	for rows.Next() {
		job, err := scanMediaJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}
	return jobs, rows.Err()
}

func (r *PostgresMediaJobRepo) Get(ctx context.Context, id string) (*coursedomain.MediaProcessingJob, error) {
	return scanMediaJob(r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM media_processing_jobs WHERE id::text=$1`, id))
}

func (r *PostgresMediaJobRepo) Cancel(ctx context.Context, id string) (*coursedomain.MediaProcessingJob, error) {
	job, err := scanMediaJob(r.pool.QueryRow(ctx, `UPDATE media_processing_jobs SET
		cancel_requested=true,status=CASE WHEN status='running' THEN status ELSE 'cancelled' END,
		progress=CASE WHEN status='running' THEN progress ELSE 0 END,
		updated_at=now(),events=events||'[{"event":"cancel_requested"}]'::jsonb
		WHERE id::text=$1 AND status IN ('pending','running','review') RETURNING `+jobColumns, id))
	if errors.Is(err, coursedomain.ErrItemNotFound) {
		return nil, coursedomain.ErrMediaJobConflict
	}
	return job, err
}

func (r *PostgresMediaJobRepo) Retry(ctx context.Context, id string) (*coursedomain.MediaProcessingJob, error) {
	job, err := scanMediaJob(r.pool.QueryRow(ctx, `UPDATE media_processing_jobs SET
		status='pending',progress=0,cancel_requested=false,lease_until=NULL,worker_id=NULL,error_code=NULL,
		updated_at=now(),events=events||'[{"event":"retried"}]'::jsonb
		WHERE id::text=$1 AND status IN ('failed','cancelled') AND attempts<3 RETURNING `+jobColumns, id))
	if errors.Is(err, coursedomain.ErrItemNotFound) {
		return nil, coursedomain.ErrMediaJobConflict
	}
	return job, err
}

func (r *PostgresMediaJobRepo) Approve(ctx context.Context, id, expectedPayloadSHA256 string) (*coursedomain.MediaProcessingJob, error) {
	job, err := scanMediaJob(r.pool.QueryRow(ctx, `UPDATE media_processing_jobs SET status='approved',progress=100,
		updated_at=now(),events=events||'[{"event":"approved"}]'::jsonb
		WHERE id::text=$1 AND status='review' AND payload_sha256=$2 RETURNING `+jobColumns, id, expectedPayloadSHA256))
	if errors.Is(err, coursedomain.ErrItemNotFound) {
		return nil, coursedomain.ErrMediaJobConflict
	}
	return job, err
}

func (r *PostgresMediaJobRepo) ReviseResult(ctx context.Context, id string, expected, replacement json.RawMessage) (*coursedomain.MediaProcessingJob, error) {
	job, err := scanMediaJob(r.pool.QueryRow(ctx, `UPDATE media_processing_jobs SET result=$3::jsonb,updated_at=now(),
		events=events||'[{"event":"timing_draft_revised"}]'::jsonb
		WHERE id::text=$1 AND status='review' AND result=$2::jsonb RETURNING `+jobColumns, id, expected, replacement))
	if errors.Is(err, coursedomain.ErrItemNotFound) {
		return nil, coursedomain.ErrMediaJobConflict
	}
	return job, err
}
