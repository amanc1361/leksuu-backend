package repo

import (
	"context"
	"encoding/json"

	"github.com/leksuu/platform/internal/modules/auth/domain"
)

func learningRecordKey(user, kind, id string) string { return user + "\x00" + kind + "\x00" + id }

func (r *MemoryUserRepo) LearningRecords(_ context.Context, user, kind string) ([]domain.LearningRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := []domain.LearningRecord{}
	for key, record := range r.learningRecords {
		prefix := user + "\x00" + kind + "\x00"
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, domain.LearningRecord{ID: record.ID, Value: append([]byte(nil), record.Value...)})
		}
	}
	return result, nil
}

func (r *MemoryUserRepo) SaveLearningRecord(_ context.Context, user, kind string, record domain.LearningRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := learningRecordKey(user, kind, record.ID)
	old, exists := r.learningRecords[key]
	if exists && (kind == "daily_mix_step" || kind == "shadow_queue" || kind == "shadow_attempt" && containsAssessment(old.Value)) {
		return nil
	}
	r.learningRecords[key] = domain.LearningRecord{ID: record.ID, Value: append([]byte(nil), record.Value...)}
	return nil
}

func containsAssessment(value []byte) bool {
	var object map[string]any
	if json.Unmarshal(value, &object) != nil {
		return false
	}
	_, ok := object["assessment"]
	return ok
}
