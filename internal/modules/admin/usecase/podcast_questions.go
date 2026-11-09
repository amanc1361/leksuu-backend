package usecase

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"

	"github.com/leksuu/platform/internal/shared/apperr"
)

// Questions bind to a snapshot of a spoken segment. They are optional payload
// metadata; adding them never rewrites audio, cues, translations or progress.
type PodcastQuestion struct {
	ID           string   `json:"id"`
	CueIndex     int      `json:"cue_index"`
	CueText      string   `json:"cue_text"`
	Start        float64  `json:"start"`
	End          float64  `json:"end"`
	Prompt       string   `json:"prompt"`
	Options      []string `json:"options"`
	CorrectIndex int      `json:"correct_index"`
	Explanation  string   `json:"explanation"`
	ReviewStatus string   `json:"review_status"`
}

type PodcastQuestionsInput struct {
	Questions         []PodcastQuestion `json:"questions"`
	ExpectedQuestions json.RawMessage   `json:"expected_questions"`
}

func (s *Service) SavePodcastQuestions(ctx context.Context, itemID string, in PodcastQuestionsInput) error {
	if len(in.Questions) > 100 || len(in.ExpectedQuestions) == 0 {
		return apperr.ErrBadRequest
	}
	item, err := s.findMediaItem(ctx, itemID)
	if err != nil {
		return err
	}
	if item.Kind != "podcast" {
		return apperr.ErrBadRequest
	}
	var payload map[string]json.RawMessage
	var expected, current any
	if json.Unmarshal(item.Payload, &payload) != nil || json.Unmarshal(in.ExpectedQuestions, &expected) != nil {
		return apperr.ErrBadRequest
	}
	if raw := payload["listening_questions"]; len(raw) > 0 && json.Unmarshal(raw, &current) != nil {
		return apperr.ErrBadRequest
	}
	if !reflect.DeepEqual(current, expected) {
		return ErrMediaJobConflict
	}
	var source struct {
		Transcript []struct {
			Text         string   `json:"text"`
			Start        *float64 `json:"start"`
			End          *float64 `json:"end"`
			TimingStatus string   `json:"timing_status"`
		} `json:"transcript"`
	}
	if json.Unmarshal(item.Payload, &source) != nil {
		return apperr.ErrBadRequest
	}
	seen := map[string]bool{}
	for _, question := range in.Questions {
		if strings.TrimSpace(question.ID) == "" || len(question.ID) > 128 || seen[question.ID] ||
			question.CueIndex < 0 || question.CueIndex >= len(source.Transcript) ||
			strings.TrimSpace(question.Prompt) == "" || len(question.Prompt) > 2000 || len(question.Explanation) > 4000 ||
			len(question.Options) < 2 || len(question.Options) > 6 || question.CorrectIndex < 0 || question.CorrectIndex >= len(question.Options) ||
			(question.ReviewStatus != "draft" && question.ReviewStatus != "approved") {
			return apperr.ErrBadRequest
		}
		seen[question.ID] = true
		cue := source.Transcript[question.CueIndex]
		if cue.Start == nil || cue.End == nil || cue.TimingStatus == "not_in_audio" ||
			math.IsNaN(question.Start) || math.IsInf(question.Start, 0) || math.IsNaN(question.End) || math.IsInf(question.End, 0) ||
			question.Start < 0 || question.End <= question.Start || question.CueText != cue.Text || question.Start != *cue.Start || question.End != *cue.End {
			return apperr.ErrBadRequest
		}
		options := map[string]bool{}
		for _, option := range question.Options {
			key := strings.ToLower(strings.TrimSpace(option))
			if key == "" || len(option) > 1000 || options[key] {
				return apperr.ErrBadRequest
			}
			options[key] = true
		}
	}
	if in.Questions == nil {
		in.Questions = []PodcastQuestion{}
	}
	questionsJSON, err := json.Marshal(in.Questions)
	if err != nil {
		return err
	}
	payload["listening_questions"] = questionsJSON
	replacement, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, itemID, item.Payload, replacement)
	if err != nil {
		return err
	}
	if !updated {
		return ErrMediaJobConflict
	}
	return nil
}
