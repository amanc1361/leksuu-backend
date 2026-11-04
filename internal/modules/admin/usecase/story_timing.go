package usecase

import (
	"context"
	"encoding/json"
	"math"
	"strings"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

var ErrStoryTimingConflict = apperr.New("admin.story_timing_conflict", apperr.KindConflict)

type StoryTimingCue struct {
	Text         string            `json:"text"`
	Start        *float64          `json:"start,omitempty"`
	End          *float64          `json:"end,omitempty"`
	TimingStatus string            `json:"timing_status,omitempty"`
	Words        []MediaWordTiming `json:"words,omitempty"`
}

type StoryTimingInput struct {
	AudioSHA256 string           `json:"audio_sha256"`
	Cues        []StoryTimingCue `json:"cues"`
}

type StoryTimingResult struct {
	ID          string `json:"id"`
	SourceID    string `json:"source_id"`
	Updated     bool   `json:"updated"`
	TimedCues   int    `json:"timed_cues"`
	UntimedCues int    `json:"untimed_cues"`
}

// BackfillStoryTiming changes only cue start/end values. The request supplies
// the exact source transcript and MP3 digest, so stale or mismatched imports
// cannot be timed. The repository's compare-and-swap protects concurrent edits.
func (s *Service) BackfillStoryTiming(ctx context.Context, sourceID string, in StoryTimingInput) (StoryTimingResult, error) {
	result := StoryTimingResult{SourceID: sourceID}
	if strings.TrimSpace(sourceID) == "" || len(in.AudioSHA256) != 64 || len(in.Cues) < 2 || len(in.Cues) > 500 {
		return result, apperr.ErrBadRequest
	}
	previousEnd := 0.0
	timedCount := 0
	for i, cue := range in.Cues {
		if cue.Text == "" {
			return result, apperr.ErrBadRequest
		}
		if cue.Start == nil || cue.End == nil {
			if cue.Start != nil || cue.End != nil || (cue.TimingStatus != "not_in_audio" && cue.TimingStatus != "unverified") {
				return result, apperr.ErrBadRequest
			}
			continue
		}
		if cue.TimingStatus != "" || math.IsNaN(*cue.Start) || math.IsNaN(*cue.End) || math.IsInf(*cue.Start, 0) || math.IsInf(*cue.End, 0) || *cue.Start < 0 || *cue.End <= *cue.Start || (i > 0 && *cue.Start < previousEnd) {
			return result, apperr.ErrBadRequest
		}
		timedCount++
		previousEnd = *cue.End
	}
	if timedCount == 0 {
		return result, apperr.ErrBadRequest
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
	var storedSHA, audioURL string
	if json.Unmarshal(payload["audio_sha256"], &storedSHA) != nil ||
		json.Unmarshal(payload["url"], &audioURL) != nil || audioURL == "" ||
		!strings.EqualFold(storedSHA, in.AudioSHA256) {
		return result, ErrStoryTimingConflict
	}
	var transcript []map[string]json.RawMessage
	if json.Unmarshal(payload["transcript"], &transcript) != nil || len(transcript) != len(in.Cues) {
		return result, ErrStoryTimingConflict
	}
	anyTimed := false
	for i, cue := range transcript {
		var text string
		if json.Unmarshal(cue["text"], &text) != nil || text != in.Cues[i].Text {
			return result, ErrStoryTimingConflict
		}
		startRaw, hasStart := cue["start"]
		endRaw, hasEnd := cue["end"]
		if !hasStart && !hasEnd {
			continue
		}
		anyTimed = true
		var start, end float64
		if !hasStart || !hasEnd || in.Cues[i].Start == nil || in.Cues[i].End == nil || json.Unmarshal(startRaw, &start) != nil || json.Unmarshal(endRaw, &end) != nil || start != *in.Cues[i].Start || end != *in.Cues[i].End {
			return result, ErrStoryTimingConflict
		}
	}
	result.TimedCues = timedCount
	result.UntimedCues = len(in.Cues) - timedCount
	if anyTimed {
		for i, cue := range transcript {
			_, hasStart := cue["start"]
			_, hasEnd := cue["end"]
			if (in.Cues[i].Start != nil) != (hasStart && hasEnd) {
				return result, ErrStoryTimingConflict
			}
			var status string
			_ = json.Unmarshal(cue["timing_status"], &status)
			if status != in.Cues[i].TimingStatus {
				return result, ErrStoryTimingConflict
			}
		}
		return result, nil
	}
	for i := range transcript {
		if in.Cues[i].Start != nil {
			start, _ := json.Marshal(*in.Cues[i].Start)
			end, _ := json.Marshal(*in.Cues[i].End)
			transcript[i]["start"] = start
			transcript[i]["end"] = end
		} else {
			status, _ := json.Marshal(in.Cues[i].TimingStatus)
			transcript[i]["timing_status"] = status
		}
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
