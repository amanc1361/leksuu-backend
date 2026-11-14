package domain

import (
	"context"
	"encoding/json"
)

type LearningRecord struct {
	ID    string          `json:"id"`
	Value json.RawMessage `json:"value"`
}

type LearningRecordRepository interface {
	LearningRecords(ctx context.Context, userID, kind string) ([]LearningRecord, error)
	SaveLearningRecord(ctx context.Context, userID, kind string, record LearningRecord) error
}
