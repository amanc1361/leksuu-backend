package repo

import (
	"context"

	"github.com/leksuu/platform/internal/modules/course/usecase"
)

func mediaProgressKey(user, item, lang string) string { return user + "|" + item + "|" + lang }

func (r *MemoryProgressRepo) LoadMediaProgress(_ context.Context, user, item, lang string) (usecase.MediaProgress, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p := r.media[mediaProgressKey(user, item, lang)]
	p.CompletedCueIDs = append([]string{}, p.CompletedCueIDs...)
	return p, nil
}

func (r *MemoryProgressRepo) SaveMediaPosition(_ context.Context, user, item, lang string, position float64, completed bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := mediaProgressKey(user, item, lang)
	p := r.media[key]
	p.PositionSeconds = position
	p.Completed = p.Completed || completed
	r.media[key] = p
	return nil
}

func (r *MemoryProgressRepo) RecordMediaAttempt(_ context.Context, user, item, lang, cue string, passed bool) (usecase.MediaProgress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := mediaProgressKey(user, item, lang)
	p := r.media[key]
	p.Attempts++
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
