package repo

import (
	"context"

	"github.com/leksuu/platform/internal/modules/course/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
)

func mediaProgressKey(user, item, lang string) string { return user + "|" + item + "|" + lang }

func (r *MemoryProgressRepo) LoadMediaProgress(_ context.Context, user, item, lang string) (usecase.MediaProgress, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p := r.media[mediaProgressKey(user, item, lang)]
	p.CompletedCueIDs = append([]string{}, p.CompletedCueIDs...)
	return p, nil
}

func (r *MemoryProgressRepo) SaveMediaPosition(_ context.Context, user, item, lang string, position float64, completed bool, revision *int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := mediaProgressKey(user, item, lang)
	p := r.media[key]
	if revision != nil && p.Revision != *revision {
		return 0, apperr.New("media.progress_conflict", apperr.KindConflict)
	}
	p.PositionSeconds = position
	p.Completed = p.Completed || completed
	p.Revision++
	r.media[key] = p
	return p.Revision, nil
}

func (r *MemoryProgressRepo) RecordMediaAttempt(_ context.Context, user, item, lang, cue string, passed bool) (usecase.MediaProgress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := mediaProgressKey(user, item, lang)
	p := r.media[key]
	p.Attempts++
	p.Revision++
	if passed {
		p.CorrectAttempts++
		found := false
		for _, id := range p.CompletedCueIDs {
			if id == cue {
				found = true
				break
			}
		}
		if !found {
			p.CompletedCueIDs = append(p.CompletedCueIDs, cue)
		}
	}
	r.media[key] = p
	p.CompletedCueIDs = append([]string{}, p.CompletedCueIDs...)
	return p, nil
}
