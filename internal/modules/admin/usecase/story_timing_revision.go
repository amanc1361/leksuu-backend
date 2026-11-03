package usecase

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
)

type StoryTimingRevisionInput struct {
	AudioSHA256    string         `json:"audio_sha256"`
	Paragraph      int            `json:"paragraph"`
	Expected       StoryTimingCue `json:"expected"`
	Replacement    StoryTimingCue `json:"replacement"`
	ReviewDecision string         `json:"review_decision,omitempty"`
	ReviewNote     string         `json:"review_note,omitempty"`
}

type StoryTimingRevisionResult struct {
	ID        string `json:"id"`
	SourceID  string `json:"source_id"`
	Paragraph int    `json:"paragraph"`
	Updated   bool   `json:"updated"`
}

func validRevisionCue(c StoryTimingCue) bool {
	if c.Text == "" {
		return false
	}
	if c.Start == nil || c.End == nil {
		return c.Start == nil && c.End == nil && (c.TimingStatus == "unverified" || c.TimingStatus == "not_in_audio")
	}
	return c.TimingStatus == "" && !math.IsNaN(*c.Start) && !math.IsNaN(*c.End) &&
		!math.IsInf(*c.Start, 0) && !math.IsInf(*c.End, 0) && *c.Start >= 0 && *c.End > *c.Start
}

func sameRevisionCue(a, b StoryTimingCue) bool {
	if a.Text != b.Text || a.TimingStatus != b.TimingStatus || (a.Start == nil) != (b.Start == nil) || (a.End == nil) != (b.End == nil) {
		return false
	}
	return (a.Start == nil || *a.Start == *b.Start) && (a.End == nil || *a.End == *b.End)
}

// ReviseStoryTiming changes exactly one cue with exact old-value, transcript,
// audio-digest, ordering and payload compare-and-swap checks. It never changes
// a confirmed not-in-audio cue through this automated recovery path.
func (s *Service) ReviseStoryTiming(ctx context.Context, sourceID string, in StoryTimingRevisionInput) (StoryTimingRevisionResult, error) {
	result := StoryTimingRevisionResult{SourceID: sourceID, Paragraph: in.Paragraph}
	if strings.TrimSpace(sourceID) == "" || len(in.AudioSHA256) != 64 || in.Paragraph < 1 || in.Paragraph > 500 ||
		!validRevisionCue(in.Expected) || !validRevisionCue(in.Replacement) || in.Expected.Text != in.Replacement.Text || len(in.ReviewNote) > 1000 {
		return result, apperr.ErrBadRequest
	}
	var reviewer string
	if in.ReviewDecision != "" {
		switch in.ReviewDecision {
		case "accepted", "corrected", "rejected", "not_in_audio":
		default:
			return result, apperr.ErrBadRequest
		}
		principal, ok := authn.FromContext(ctx)
		if !ok || !principal.HasRole("admin") || principal.Subject == "" {
			return result, apperr.New("admin.review_forbidden", apperr.KindForbidden)
		}
		reviewer = principal.Subject
		if (in.ReviewDecision == "accepted" || in.ReviewDecision == "corrected") && in.Replacement.Start == nil {
			return result, apperr.ErrBadRequest
		}
		if in.ReviewDecision == "not_in_audio" && in.Replacement.TimingStatus != "not_in_audio" {
			return result, apperr.ErrBadRequest
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
				return result, ErrStoryTimingConflict
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
		return result, ErrStoryTimingConflict
	}
	var transcript []map[string]json.RawMessage
	if json.Unmarshal(payload["transcript"], &transcript) != nil || in.Paragraph > len(transcript) {
		return result, ErrStoryTimingConflict
	}
	index := in.Paragraph - 1
	parse := func(c map[string]json.RawMessage) (StoryTimingCue, error) {
		var out StoryTimingCue
		data, err := json.Marshal(c)
		if err != nil {
			return out, err
		}
		err = json.Unmarshal(data, &out)
		return out, err
	}
	current, err := parse(transcript[index])
	if err != nil || current.Text != in.Expected.Text {
		return result, ErrStoryTimingConflict
	}
	if in.ReviewDecision == "rejected" && !sameRevisionCue(current, in.Replacement) {
		return result, apperr.ErrBadRequest
	}
	if sameRevisionCue(current, in.Replacement) && in.ReviewDecision == "" {
		return result, nil
	}
	if !sameRevisionCue(current, in.Expected) || current.TimingStatus == "not_in_audio" {
		return result, ErrStoryTimingConflict
	}
	if in.Replacement.Start != nil {
		for i, cue := range transcript {
			if i == index {
				continue
			}
			other, err := parse(cue)
			if err != nil {
				return result, ErrStoryTimingConflict
			}
			if other.Start == nil || other.End == nil {
				continue
			}
			if (i < index && *other.End > *in.Replacement.Start) || (i > index && *other.Start < *in.Replacement.End) {
				return result, apperr.ErrBadRequest
			}
		}
	}
	delete(transcript[index], "start")
	delete(transcript[index], "end")
	delete(transcript[index], "timing_status")
	if in.Replacement.Start != nil {
		transcript[index]["start"], _ = json.Marshal(in.Replacement.Start)
		transcript[index]["end"], _ = json.Marshal(in.Replacement.End)
	} else {
		transcript[index]["timing_status"], _ = json.Marshal(in.Replacement.TimingStatus)
	}
	if reviewer != "" {
		var history []map[string]any
		if raw := transcript[index]["editorial_review_history"]; len(raw) > 0 && json.Unmarshal(raw, &history) != nil {
			return result, ErrStoryTimingConflict
		}
		entry := map[string]any{"decision": in.ReviewDecision, "reviewer_id": reviewer,
			"at": time.Now().UTC().Format(time.RFC3339Nano), "note": strings.TrimSpace(in.ReviewNote)}
		history = append(history, entry)
		transcript[index]["editorial_review_history"], _ = json.Marshal(history)
		transcript[index]["editorial_review"], _ = json.Marshal(entry)
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
		return result, ErrStoryTimingConflict
	}
	result.Updated = true
	return result, nil
}
