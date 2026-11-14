//go:build pgx

package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/leksuu/platform/internal/modules/course/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
)

func (r *PostgresProgressRepo) LoadMediaProgress(ctx context.Context, user, item, lang string) (usecase.MediaProgress, error) {
	var p usecase.MediaProgress
	var cues []byte
	err := r.pool.QueryRow(ctx, `SELECT position_seconds,attempts,correct_attempts,completed_cue_ids,completed,revision
		FROM media_learner_progress WHERE user_id=$1 AND item_id=$2 AND target_language=$3`, user, item, lang).
		Scan(&p.PositionSeconds, &p.Attempts, &p.CorrectAttempts, &cues, &p.Completed, &p.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(cues, &p.CompletedCueIDs); err != nil {
		return p, err
	}
	return p, nil
}

func (r *PostgresProgressRepo) SaveMediaPosition(ctx context.Context, user, item, lang string, position float64, completed bool, revision *int64) (int64, error) {
	var next int64
	err := r.pool.QueryRow(ctx, `INSERT INTO media_learner_progress(user_id,item_id,target_language,position_seconds,completed,revision)
		SELECT $1,$2,$3,$4,$5,1 WHERE $6::bigint IS NULL OR $6=0 OR EXISTS (
			SELECT 1 FROM media_learner_progress WHERE user_id=$1 AND item_id=$2 AND target_language=$3)
		ON CONFLICT(user_id,item_id,target_language) DO UPDATE
		SET position_seconds=EXCLUDED.position_seconds,
		completed=media_learner_progress.completed OR EXCLUDED.completed,
		revision=media_learner_progress.revision+1,updated_at=now()
		WHERE $6::bigint IS NULL OR media_learner_progress.revision=$6
		RETURNING revision`, user, item, lang, position, completed, revision).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, apperr.New("media.progress_conflict", apperr.KindConflict)
	}
	return next, err
}

func (r *PostgresProgressRepo) RecordMediaAttempt(ctx context.Context, user, item, lang, cue string, passed bool) (usecase.MediaProgress, error) {
	var p usecase.MediaProgress
	var cues []byte
	err := r.pool.QueryRow(ctx, `INSERT INTO media_learner_progress(user_id,item_id,target_language,attempts,correct_attempts,completed_cue_ids,revision)
		VALUES($1,$2,$3,1,CASE WHEN $5 THEN 1 ELSE 0 END,CASE WHEN $5 THEN jsonb_build_array($4::text) ELSE '[]'::jsonb END,1)
		ON CONFLICT(user_id,item_id,target_language) DO UPDATE SET
		attempts=media_learner_progress.attempts+1,
		correct_attempts=media_learner_progress.correct_attempts+CASE WHEN $5 THEN 1 ELSE 0 END,
		completed_cue_ids=CASE WHEN NOT $5 OR media_learner_progress.completed_cue_ids ? $4 THEN media_learner_progress.completed_cue_ids
		ELSE media_learner_progress.completed_cue_ids || jsonb_build_array($4::text) END,
		revision=media_learner_progress.revision+1,updated_at=now()
		RETURNING position_seconds,attempts,correct_attempts,completed_cue_ids,completed,revision`, user, item, lang, cue, passed).
		Scan(&p.PositionSeconds, &p.Attempts, &p.CorrectAttempts, &cues, &p.Completed, &p.Revision)
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(cues, &p.CompletedCueIDs); err != nil {
		return p, err
	}
	return p, nil
}
