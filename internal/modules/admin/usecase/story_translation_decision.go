package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
)

type StoryTranslationDecisionInput struct {
	AudioSHA256 string `json:"audio_sha256"`
	Paragraph   int    `json:"paragraph"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	Expected    string `json:"expected"`
	Decision    string `json:"decision"`
	Note        string `json:"note"`
}

type StoryTranslationDecisionResult struct {
	ID        string `json:"id"`
	SourceID  string `json:"source_id"`
	Paragraph int    `json:"paragraph"`
	Language  string `json:"language"`
	Decision  string `json:"decision"`
	Updated   bool   `json:"updated"`
}

// DecideStoryTranslation records an authenticated human decision on exactly
// one cue. Generated drafts never call this method.
func (s *Service) DecideStoryTranslation(ctx context.Context, sourceID string, in StoryTranslationDecisionInput) (StoryTranslationDecisionResult, error) {
	result := StoryTranslationDecisionResult{SourceID: sourceID, Paragraph: in.Paragraph, Language: in.Language, Decision: in.Decision}
	principal, ok := authn.FromContext(ctx)
	if !ok || !principal.HasRole("admin") || principal.Subject == "" {
		return result, apperr.New("admin.review_forbidden", apperr.KindForbidden)
	}
	if (in.Language != "fa" && in.Language != "ckb") || (in.Decision != "approved" && in.Decision != "rejected") ||
		in.Paragraph < 1 || len(in.Note) > 1000 {
		return result, apperr.ErrBadRequest
	}
	item, payload, cues, err := s.translationTarget(ctx, sourceID, in.AudioSHA256)
	if err != nil {
		return result, err
	}
	result.ID = item.ID
	if in.Paragraph > len(cues) {
		return result, apperr.ErrBadRequest
	}
	cue := cues[in.Paragraph-1]
	var text string
	var translation map[string]string
	var reviews map[string]string
	if json.Unmarshal(cue["text"], &text) != nil || text != in.Text ||
		json.Unmarshal(cue["translation"], &translation) != nil ||
		strings.TrimSpace(translation[in.Language]) == "" || translation[in.Language] != in.Expected {
		return result, ErrStoryTranslationConflict
	}
	if raw := cue["translation_review"]; len(raw) > 0 && json.Unmarshal(raw, &reviews) != nil {
		return result, ErrStoryTranslationConflict
	}
	if reviews == nil {
		reviews = map[string]string{}
	}
	if reviews[in.Language] == in.Decision {
		return result, nil
	}
	reviews[in.Language] = in.Decision
	cue["translation_review"], _ = json.Marshal(reviews)
	var history []map[string]any
	if raw := cue["translation_review_history"]; len(raw) > 0 && json.Unmarshal(raw, &history) != nil {
		return result, ErrStoryTranslationConflict
	}
	history = append(history, map[string]any{"language": in.Language, "decision": in.Decision,
		"reviewer_id": principal.Subject, "at": time.Now().UTC().Format(time.RFC3339Nano),
		"note": strings.TrimSpace(in.Note)})
	cue["translation_review_history"], _ = json.Marshal(history)
	var statuses map[string]string
	if raw := payload["translation_status"]; len(raw) > 0 && json.Unmarshal(raw, &statuses) != nil {
		return result, ErrStoryTranslationConflict
	}
	if statuses == nil {
		statuses = map[string]string{}
	}
	allApproved := true
	for _, entry := range cues {
		var state map[string]string
		_ = json.Unmarshal(entry["translation_review"], &state)
		if state[in.Language] != "approved" {
			allApproved = false
			break
		}
	}
	if allApproved {
		statuses[in.Language] = "approved"
	} else {
		statuses[in.Language] = "draft"
	}
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
