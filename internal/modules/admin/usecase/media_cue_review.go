package usecase

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
)

type MediaCueReviewInput struct {
	CueID            string            `json:"cue_id"`
	ExpectedRevision int               `json:"expected_revision"`
	Action           string            `json:"action"`
	Text             string            `json:"text"`
	Start            *float64          `json:"start"`
	End              *float64          `json:"end"`
	Translation      map[string]string `json:"translation"`
	Note             string            `json:"note"`
}

// ReviewMediaCue edits or decides one stable cue. Every mutation uses a payload
// compare-and-swap, so concurrent reviewers cannot silently overwrite each other.
func (s *Service) ReviewMediaCue(ctx context.Context, itemID string, in MediaCueReviewInput) error {
	p, ok := authn.FromContext(ctx)
	if !ok || !p.HasRole("admin") || p.Subject == "" {
		return apperr.New("admin.review_forbidden", apperr.KindForbidden)
	}
	if in.CueID == "" || len(in.CueID) > 200 || len(in.Note) > 1000 || in.ExpectedRevision < 0 {
		return apperr.ErrBadRequest
	}
	item, err := s.findMediaItem(ctx, itemID)
	if err != nil {
		return err
	}
	if item.Kind != "video" && item.Kind != "music" {
		return apperr.ErrBadRequest
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(item.Payload, &payload) != nil {
		return apperr.ErrBadRequest
	}
	var published bool
	_ = json.Unmarshal(payload["published"], &published)
	if published {
		return ErrMediaPublicationReview
	}
	var cues []map[string]json.RawMessage
	if json.Unmarshal(payload["transcript"], &cues) != nil {
		return apperr.ErrBadRequest
	}
	index := -1
	for i, cue := range cues {
		var id string
		_ = json.Unmarshal(cue["cue_id"], &id)
		if id == in.CueID {
			if index >= 0 {
				return ErrMediaJobConflict
			}
			index = i
		}
	}
	if index < 0 {
		return apperr.ErrBadRequest
	}
	cue := cues[index]
	var revision int
	_ = json.Unmarshal(cue["review_revision"], &revision)
	if revision != in.ExpectedRevision {
		return ErrMediaJobConflict
	}
	var sourceReview struct {
		Decision string `json:"decision"`
	}
	_ = json.Unmarshal(cue["editorial_review"], &sourceReview)
	var translations map[string]string
	_ = json.Unmarshal(cue["translation"], &translations)
	if translations == nil {
		translations = map[string]string{}
	}
	var translationReview map[string]string
	_ = json.Unmarshal(cue["translation_review"], &translationReview)
	if translationReview == nil {
		translationReview = map[string]string{}
	}
	var exerciseReview string
	_ = json.Unmarshal(cue["exercise_review"], &exerciseReview)
	var beforeText string
	_ = json.Unmarshal(cue["text"], &beforeText)
	var start, end float64
	_ = json.Unmarshal(cue["start"], &start)
	_ = json.Unmarshal(cue["end"], &end)
	write := func(key string, v any) { cue[key], _ = json.Marshal(v) }
	switch in.Action {
	case "edit":
		if in.Start == nil || in.End == nil || math.IsNaN(*in.Start) || math.IsNaN(*in.End) ||
			math.IsInf(*in.Start, 0) || math.IsInf(*in.End, 0) || *in.Start < 0 || *in.End <= *in.Start || *in.End > 864000 ||
			strings.TrimSpace(in.Text) == "" || len(in.Text) > 10000 {
			return apperr.ErrBadRequest
		}
		sourceChanged := beforeText != strings.TrimSpace(in.Text) || start != *in.Start || end != *in.End
		if sourceReview.Decision == "approved" && sourceChanged {
			return ErrMediaPublicationReview
		}
		for _, lang := range []string{"fa", "ckb"} {
			if value, exists := in.Translation[lang]; exists {
				if translationReview[lang] == "approved" && translations[lang] != value {
					return ErrMediaPublicationReview
				}
				if len(value) > 10000 {
					return apperr.ErrBadRequest
				}
				if translations[lang] != strings.TrimSpace(value) {
					translationReview[lang] = "draft"
				}
				translations[lang] = strings.TrimSpace(value)
			}
		}
		if exerciseReview == "approved" && sourceChanged {
			return ErrMediaPublicationReview
		}
		write("text", strings.TrimSpace(in.Text))
		write("start", *in.Start)
		write("end", *in.End)
		write("translation", translations)
		write("translation_review", translationReview)
		if sourceChanged {
			write("timing_status", "needs_review")
			write("editorial_review", map[string]string{"decision": "draft"})
			write("exercise_review", "draft")
			for _, lang := range []string{"fa", "ckb"} {
				translationReview[lang] = "draft"
			}
			write("translation_review", translationReview)
		}
	case "source_approve":
		if strings.TrimSpace(beforeText) == "" || end <= start {
			return apperr.ErrBadRequest
		}
		write("timing_status", "approved")
		write("editorial_review", map[string]string{"decision": "approved"})
	case "fa_approve", "ckb_approve":
		lang := strings.TrimSuffix(in.Action, "_approve")
		if strings.TrimSpace(translations[lang]) == "" {
			return apperr.ErrBadRequest
		}
		translationReview[lang] = "approved"
		write("translation_review", translationReview)
	case "exercise_approve":
		if sourceReview.Decision != "approved" || len(strings.Fields(beforeText)) < 2 {
			return apperr.ErrBadRequest
		}
		write("exercise_review", "approved")
	case "source_reopen":
		write("timing_status", "needs_review")
		write("editorial_review", map[string]string{"decision": "draft"})
		write("exercise_review", "draft")
	case "fa_reopen", "ckb_reopen":
		lang := strings.TrimSuffix(in.Action, "_reopen")
		translationReview[lang] = "draft"
		write("translation_review", translationReview)
	case "exercise_reopen":
		write("exercise_review", "draft")
	default:
		return apperr.ErrBadRequest
	}
	if in.Action == "edit" || in.Action == "source_approve" {
		var currentStart, currentEnd float64
		_ = json.Unmarshal(cue["start"], &currentStart)
		_ = json.Unmarshal(cue["end"], &currentEnd)
		if index > 0 {
			var previousEnd float64
			if json.Unmarshal(cues[index-1]["end"], &previousEnd) == nil && currentStart < previousEnd {
				return apperr.ErrBadRequest
			}
		}
		if index+1 < len(cues) {
			var nextStart float64
			if json.Unmarshal(cues[index+1]["start"], &nextStart) == nil && currentEnd > nextStart {
				return apperr.ErrBadRequest
			}
		}
	}
	write("review_revision", revision+1)
	var history []map[string]any
	_ = json.Unmarshal(cue["review_history"], &history)
	history = append(history, map[string]any{"action": in.Action, "reviewer_id": p.Subject, "at": time.Now().UTC().Format(time.RFC3339Nano), "note": strings.TrimSpace(in.Note), "revision": revision + 1})
	write("review_history", history)
	for _, lang := range []string{"fa", "ckb"} {
		all := true
		for _, entry := range cues {
			var reviews map[string]string
			_ = json.Unmarshal(entry["translation_review"], &reviews)
			if reviews[lang] != "approved" {
				all = false
				break
			}
		}
		var statuses map[string]string
		_ = json.Unmarshal(payload["translation_status"], &statuses)
		if statuses == nil {
			statuses = map[string]string{}
		}
		if all {
			statuses[lang] = "approved"
		} else {
			statuses[lang] = "draft"
		}
		payload["translation_status"], _ = json.Marshal(statuses)
	}
	payload["transcript"], _ = json.Marshal(cues)
	replacement, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, item.ID, item.Payload, replacement)
	if err != nil {
		return err
	}
	if !updated {
		return ErrMediaJobConflict
	}
	return nil
}
