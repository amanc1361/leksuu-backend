package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

var ErrStoryTranslationConflict = apperr.New("admin.story_translation_conflict", apperr.KindConflict)

type StoryTranslationCue struct {
	Text        string            `json:"text"`
	Translation map[string]string `json:"translation"`
}

type StoryTranslationInput struct {
	AudioSHA256 string                `json:"audio_sha256"`
	Cues        []StoryTranslationCue `json:"cues"`
}

type StoryTranslationResult struct {
	ID       string         `json:"id"`
	SourceID string         `json:"source_id"`
	Updated  bool           `json:"updated"`
	Filled   map[string]int `json:"filled"`
}

// BackfillStoryTranslations fills missing fa/ckb text on the exact source
// transcript. Existing translations, timing, publication and media stay intact.
// Generated text is marked draft; it is never marked human-approved here.
func (s *Service) BackfillStoryTranslations(ctx context.Context, sourceID string, in StoryTranslationInput) (StoryTranslationResult, error) {
	result := StoryTranslationResult{SourceID: sourceID, Filled: map[string]int{"fa": 0, "ckb": 0}}
	if strings.TrimSpace(sourceID) == "" || len(in.AudioSHA256) != 64 || len(in.Cues) < 2 || len(in.Cues) > 500 {
		return result, apperr.ErrBadRequest
	}
	provided := map[string]bool{}
	for _, cue := range in.Cues {
		if cue.Text == "" {
			return result, apperr.ErrBadRequest
		}
		for lang, value := range cue.Translation {
			if lang != "fa" && lang != "ckb" {
				return result, apperr.ErrBadRequest
			}
			if strings.TrimSpace(value) == "" || len(value) > 10000 || !hasArabicScript(value) {
				return result, apperr.ErrBadRequest
			}
			provided[lang] = true
		}
	}
	if len(provided) == 0 {
		return result, apperr.ErrBadRequest
	}
	for lang := range provided {
		for _, cue := range in.Cues {
			if cue.Translation[lang] == "" {
				return result, apperr.ErrBadRequest
			}
		}
	}
	items, err := s.AdminMedia(ctx, coursedomain.KindStory)
	if err != nil {
		return result, err
	}
	var match *coursedomain.LessonItem
	for i := range items {
		var identity struct {
			SourceID string `json:"source_id"`
		}
		if json.Unmarshal(items[i].Payload, &identity) == nil && identity.SourceID == sourceID {
			if match != nil {
				return result, ErrStoryTranslationConflict
			}
			match = &items[i]
		}
	}
	if match == nil {
		return result, coursedomain.ErrItemNotFound
	}
	result.ID = match.ID
	payload := map[string]json.RawMessage{}
	if json.Unmarshal(match.Payload, &payload) != nil {
		return result, apperr.ErrInternal
	}
	var storedSHA string
	if json.Unmarshal(payload["audio_sha256"], &storedSHA) != nil || !strings.EqualFold(storedSHA, in.AudioSHA256) {
		return result, ErrStoryTranslationConflict
	}
	var transcript []map[string]json.RawMessage
	if json.Unmarshal(payload["transcript"], &transcript) != nil || len(transcript) != len(in.Cues) {
		return result, ErrStoryTranslationConflict
	}
	var statuses map[string]string
	if raw := payload["translation_status"]; len(raw) > 0 && json.Unmarshal(raw, &statuses) != nil {
		return result, ErrStoryTranslationConflict
	}
	if statuses == nil {
		statuses = map[string]string{}
	}
	changed := false
	for i, cue := range transcript {
		var original string
		if json.Unmarshal(cue["text"], &original) != nil || original != in.Cues[i].Text {
			return result, ErrStoryTranslationConflict
		}
		var existing map[string]string
		if raw := cue["translation"]; len(raw) > 0 && json.Unmarshal(raw, &existing) != nil {
			return result, ErrStoryTranslationConflict
		}
		if existing == nil {
			existing = map[string]string{}
		}
		for lang := range provided {
			incoming := in.Cues[i].Translation[lang]
			if current := existing[lang]; current != "" && current != incoming {
				return result, ErrStoryTranslationConflict
			} else if current == "" {
				if statuses[lang] == "approved" || statuses[lang] == "human_approved" {
					return result, ErrStoryTranslationConflict
				}
				existing[lang] = incoming
				result.Filled[lang]++
				changed = true
			}
		}
		if changed {
			cue["translation"], err = json.Marshal(existing)
			if err != nil {
				return result, err
			}
		}
	}
	if !changed {
		return result, nil
	}
	for lang := range provided {
		if statuses[lang] == "" {
			statuses[lang] = "draft"
		}
	}
	payload["translation_status"], err = json.Marshal(statuses)
	if err != nil {
		return result, err
	}
	payload["transcript"], err = json.Marshal(transcript)
	if err != nil {
		return result, err
	}
	replacement, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, match.ID, match.Payload, replacement)
	if err != nil {
		return result, err
	}
	if !updated {
		return result, ErrStoryTranslationConflict
	}
	result.Updated = true
	return result, nil
}

func hasArabicScript(text string) bool {
	for _, r := range text {
		if unicode.In(r, unicode.Arabic) {
			return true
		}
	}
	return false
}
