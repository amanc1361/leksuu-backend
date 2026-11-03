package usecase

import (
	"context"
	"encoding/json"
	"strings"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

type StoryTranslationRevisionInput struct {
	AudioSHA256 string `json:"audio_sha256"`
	Paragraph   int    `json:"paragraph"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	Expected    string `json:"expected"`
	Replacement string `json:"replacement"`
}

type StoryTranslationApprovalInput struct {
	AudioSHA256 string `json:"audio_sha256"`
	Language    string `json:"language"`
}

func (s *Service) translationTarget(ctx context.Context, sourceID, audioSHA string) (*coursedomain.LessonItem, map[string]json.RawMessage, []map[string]json.RawMessage, error) {
	if sourceID == "" || len(audioSHA) != 64 {
		return nil, nil, nil, apperr.ErrBadRequest
	}
	items, err := s.AdminMedia(ctx, "")
	if err != nil {
		return nil, nil, nil, err
	}
	var item *coursedomain.LessonItem
	for i := range items {
		var identity struct {
			SourceID string `json:"source_id"`
		}
		if json.Unmarshal(items[i].Payload, &identity) == nil && (identity.SourceID == sourceID || items[i].ID == sourceID) {
			if item != nil {
				return nil, nil, nil, ErrStoryTranslationConflict
			}
			item = &items[i]
		}
	}
	if item == nil {
		return nil, nil, nil, coursedomain.ErrItemNotFound
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(item.Payload, &payload) != nil {
		return nil, nil, nil, apperr.ErrInternal
	}
	var storedSHA string
	if json.Unmarshal(payload["audio_sha256"], &storedSHA) != nil || !strings.EqualFold(storedSHA, audioSHA) {
		return nil, nil, nil, ErrStoryTranslationConflict
	}
	var cues []map[string]json.RawMessage
	if json.Unmarshal(payload["transcript"], &cues) != nil || len(cues) == 0 {
		return nil, nil, nil, ErrStoryTranslationConflict
	}
	return item, payload, cues, nil
}

func (s *Service) ReviseStoryTranslation(ctx context.Context, sourceID string, in StoryTranslationRevisionInput) (StoryTranslationResult, error) {
	result := StoryTranslationResult{SourceID: sourceID, Filled: map[string]int{"fa": 0, "ckb": 0}}
	if in.Language != "fa" && in.Language != "ckb" {
		return result, apperr.ErrBadRequest
	}
	if len(in.Replacement) > 10000 || !hasArabicScript(in.Replacement) {
		return result, apperr.ErrBadRequest
	}
	item, payload, cues, err := s.translationTarget(ctx, sourceID, in.AudioSHA256)
	if err != nil {
		return result, err
	}
	result.ID = item.ID
	if in.Paragraph < 1 || in.Paragraph > len(cues) {
		return result, apperr.ErrBadRequest
	}
	var text string
	cue := cues[in.Paragraph-1]
	if json.Unmarshal(cue["text"], &text) != nil || text != in.Text {
		return result, ErrStoryTranslationConflict
	}
	var translation map[string]string
	if raw := cue["translation"]; len(raw) > 0 && json.Unmarshal(raw, &translation) != nil {
		return result, ErrStoryTranslationConflict
	}
	if translation == nil {
		translation = map[string]string{}
	}
	if translation[in.Language] != in.Expected {
		return result, ErrStoryTranslationConflict
	}
	var statuses map[string]string
	if raw := payload["translation_status"]; len(raw) > 0 && json.Unmarshal(raw, &statuses) != nil {
		return result, ErrStoryTranslationConflict
	}
	if statuses[in.Language] == "approved" || statuses[in.Language] == "human_approved" {
		return result, ErrStoryTranslationConflict
	}
	var review map[string]string
	if raw := cue["translation_review"]; len(raw) > 0 && json.Unmarshal(raw, &review) != nil {
		return result, ErrStoryTranslationConflict
	}
	if review[in.Language] == "approved" {
		return result, ErrStoryTranslationConflict
	}
	if in.Expected == in.Replacement {
		return result, nil
	}
	translation[in.Language] = in.Replacement
	cue["translation"], _ = json.Marshal(translation)
	if review != nil && review[in.Language] == "rejected" {
		review[in.Language] = "draft"
		cue["translation_review"], _ = json.Marshal(review)
	}
	if statuses == nil {
		statuses = map[string]string{}
	}
	statuses[in.Language] = "draft"
	payload["translation_status"], _ = json.Marshal(statuses)
	payload["transcript"], _ = json.Marshal(cues)
	replacement, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, item.ID, item.Payload, replacement)
	if err != nil {
		return result, err
	}
	if !updated {
		return result, ErrStoryTranslationConflict
	}
	result.Updated = true
	return result, nil
}

func (s *Service) ApproveStoryTranslations(ctx context.Context, sourceID string, in StoryTranslationApprovalInput) (StoryTranslationResult, error) {
	result := StoryTranslationResult{SourceID: sourceID, Filled: map[string]int{"fa": 0, "ckb": 0}}
	if in.Language != "fa" && in.Language != "ckb" {
		return result, apperr.ErrBadRequest
	}
	item, payload, cues, err := s.translationTarget(ctx, sourceID, in.AudioSHA256)
	if err != nil {
		return result, err
	}
	result.ID = item.ID
	var statuses map[string]string
	if raw := payload["translation_status"]; len(raw) > 0 && json.Unmarshal(raw, &statuses) != nil {
		return result, ErrStoryTranslationConflict
	}
	if statuses == nil {
		statuses = map[string]string{}
	}
	if statuses[in.Language] == "approved" || statuses[in.Language] == "human_approved" {
		return result, nil
	}
	for _, cue := range cues {
		var translations map[string]string
		if json.Unmarshal(cue["translation"], &translations) != nil || strings.TrimSpace(translations[in.Language]) == "" {
			return result, ErrStoryTranslationConflict
		}
		var review map[string]string
		_ = json.Unmarshal(cue["translation_review"], &review)
		if review[in.Language] != "approved" {
			return result, ErrStoryTranslationConflict
		}
		result.Filled[in.Language]++
	}
	statuses[in.Language] = "approved"
	payload["translation_status"], _ = json.Marshal(statuses)
	replacement, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, item.ID, item.Payload, replacement)
	if err != nil {
		return result, err
	}
	if !updated {
		return result, ErrStoryTranslationConflict
	}
	result.Updated = true
	return result, nil
}
