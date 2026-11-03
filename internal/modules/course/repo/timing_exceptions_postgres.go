//go:build pgx

package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
)

func (r *PostgresCourseRepo) IngestTimingExceptions(ctx context.Context, records []coursedomain.TimingExceptionQueueRecord) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	// Serialize report ingests so a concurrent conflicting insert is never silently skipped.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(590059)`); err != nil {
		return 0, err
	}
	added := 0
	for _, record := range records {
		var previous string
		err := tx.QueryRow(ctx, `SELECT record_sha256 FROM admin_timing_exceptions WHERE source_id=$1 AND paragraph=$2 FOR UPDATE`,
			record.SourceID, record.Paragraph).Scan(&previous)
		if err == nil {
			if previous != record.RecordSHA256 {
				return 0, coursedomain.ErrTimingExceptionConflict
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO admin_timing_exceptions(source_id,paragraph,audio_sha256,transcript_sha256,record_sha256,record)
            VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT DO NOTHING`, record.SourceID, record.Paragraph,
			record.AudioSHA256, record.TranscriptSHA256, record.RecordSHA256, record.Record)
		if err != nil {
			return 0, err
		}
		added++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return added, nil
}

func (r *PostgresCourseRepo) ListTimingExceptions(ctx context.Context) ([]coursedomain.TimingExceptionQueueRecord, error) {
	rows, err := r.pool.Query(ctx, `SELECT source_id,paragraph,audio_sha256,transcript_sha256,record_sha256,record::text
        FROM admin_timing_exceptions ORDER BY source_id,paragraph`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]coursedomain.TimingExceptionQueueRecord, 0)
	for rows.Next() {
		var record coursedomain.TimingExceptionQueueRecord
		if err := rows.Scan(&record.SourceID, &record.Paragraph, &record.AudioSHA256, &record.TranscriptSHA256,
			&record.RecordSHA256, &record.Record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
