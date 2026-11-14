package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

func learningKindOK(kind string) bool {
	return kind == "shadow_queue" || kind == "shadow_attempt" || kind == "daily_mix_step" || kind == "course_location"
}

func (s *Service) LearningRecords(ctx context.Context, user, kind string) ([]domain.LearningRecord, error) {
	if user == "" || !learningKindOK(kind) {
		return nil, apperr.ErrBadRequest
	}
	return s.users.LearningRecords(ctx, user, kind)
}

func (s *Service) SaveLearningRecord(ctx context.Context, user, kind string, record domain.LearningRecord) error {
	if user == "" || !learningKindOK(kind) || len(record.ID) == 0 || len(record.ID) > 512 || strings.ContainsRune(record.ID, 0) || len(record.Value) == 0 || len(record.Value) > 8192 {
		return apperr.ErrBadRequest
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(record.Value, &value) != nil || value == nil {
		return apperr.ErrBadRequest
	}
	if kind == "daily_mix_step" {
		parts := strings.Split(record.ID, ":")
		if len(parts) != 2 || parts[0] != time.Now().UTC().Format("2006-01-02") || (parts[1] != "shadow" && parts[1] != "media" && parts[1] != "story") {
			return apperr.ErrBadRequest
		}
		var done bool
		if json.Unmarshal(value["done"], &done) != nil || !done {
			return apperr.ErrBadRequest
		}
	} else if kind == "course_location" {
		var valueID, courseID, lessonID string
		if json.Unmarshal(value["id"], &valueID) != nil || valueID != record.ID ||
			json.Unmarshal(value["courseId"], &courseID) != nil || courseID == "" ||
			json.Unmarshal(value["lessonId"], &lessonID) != nil || lessonID == "" {
			return apperr.ErrBadRequest
		}
	} else {
		var valueID string
		if json.Unmarshal(value["id"], &valueID) != nil || valueID != record.ID || value["recording"] != nil || value["audio"] != nil || value["blob"] != nil {
			return apperr.ErrBadRequest
		}
	}
	return s.users.SaveLearningRecord(ctx, user, kind, record)
}
