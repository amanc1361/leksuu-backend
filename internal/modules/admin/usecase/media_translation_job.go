package usecase

import (
	"context"
	"encoding/json"
	"strings"
)

type translationJobCue struct {
	Paragraph   int    `json:"paragraph"`
	Text        string `json:"text"`
	Translation string `json:"translation"`
}

// applyTranslationJob accepts generated text as an unapproved draft. A
// separate, authenticated native-review action must approve each translation.
func (s *Service) applyTranslationJob(ctx context.Context, job *MediaProcessingJob) (*MediaProcessingJob, error) {
	item, err := s.findMediaItem(ctx, job.ItemID)
	if err != nil {
		return nil, err
	}
	hash, err := translationSourceHash(item.Payload)
	if err != nil || hash != job.PayloadSHA256 {
		return nil, ErrMediaJobConflict
	}
	var result struct {
		TargetLanguage string              `json:"target_language"`
		SourceSHA256   string              `json:"source_sha256"`
		Translations   []translationJobCue `json:"translations"`
	}
	if json.Unmarshal(job.Result, &result) != nil || result.TargetLanguage != job.TargetLanguage ||
		result.SourceSHA256 != job.PayloadSHA256 || len(result.Translations) == 0 {
		return nil, ErrMediaJobConflict
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(item.Payload, &payload) != nil {
		return nil, ErrMediaJobConflict
	}
	var cues []map[string]json.RawMessage
	if json.Unmarshal(payload["transcript"], &cues) != nil {
		return nil, ErrMediaJobConflict
	}
	var statuses map[string]string
	if raw := payload["translation_status"]; len(raw) > 0 && json.Unmarshal(raw, &statuses) != nil {
		return nil, ErrMediaJobConflict
	}
	if statuses == nil {
		statuses = map[string]string{}
	}
	if statuses[job.TargetLanguage] == "approved" || statuses[job.TargetLanguage] == "human_approved" {
		return nil, ErrMediaJobConflict
	}
	seen := map[int]bool{}
	changed := false
	for _, proposed := range result.Translations {
		if proposed.Paragraph < 1 || proposed.Paragraph > len(cues) || seen[proposed.Paragraph] ||
			strings.TrimSpace(proposed.Translation) == "" || len(proposed.Translation) > 10000 {
			return nil, ErrMediaJobConflict
		}
		seen[proposed.Paragraph] = true
		cue := cues[proposed.Paragraph-1]
		var text string
		if json.Unmarshal(cue["text"], &text) != nil || text != proposed.Text {
			return nil, ErrMediaJobConflict
		}
		var translations map[string]string
		if raw := cue["translation"]; len(raw) > 0 && json.Unmarshal(raw, &translations) != nil {
			return nil, ErrMediaJobConflict
		}
		if translations == nil {
			translations = map[string]string{}
		}
		var review map[string]string
		if raw := cue["translation_review"]; len(raw) > 0 && json.Unmarshal(raw, &review) != nil {
			return nil, ErrMediaJobConflict
		}
		if review[job.TargetLanguage] == "approved" {
			return nil, ErrMediaJobConflict
		}
		if existing := strings.TrimSpace(translations[job.TargetLanguage]); existing != "" {
			if existing != proposed.Translation {
				return nil, ErrMediaJobConflict
			}
			continue
		}
		translations[job.TargetLanguage] = proposed.Translation
		cue["translation"], _ = json.Marshal(translations)
		changed = true
	}
	for _, cue := range cues {
		var translations map[string]string
		if raw := cue["translation"]; len(raw) > 0 && json.Unmarshal(raw, &translations) != nil {
			return nil, ErrMediaJobConflict
		}
		if strings.TrimSpace(translations[job.TargetLanguage]) == "" {
			return nil, ErrMediaJobConflict
		}
	}
	if changed {
		statuses[job.TargetLanguage] = "draft"
		payload["translation_status"], _ = json.Marshal(statuses)
		payload["transcript"], _ = json.Marshal(cues)
		replacement, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, marshalErr
		}
		updated, updateErr := s.repo.UpdateItemPayloadIfUnchanged(ctx, item.ID, item.Payload, replacement)
		if updateErr != nil {
			return nil, updateErr
		}
		if !updated {
			return nil, ErrMediaJobConflict
		}
	}
	return s.mediaJobs.Approve(ctx, job.ID, job.PayloadSHA256)
}
