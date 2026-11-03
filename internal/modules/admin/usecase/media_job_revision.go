package usecase

import (
	"context"
	"encoding/json"
	"math"
)

type MediaJobCueRevision struct {
	Paragraph     int      `json:"paragraph"`
	Text          string   `json:"text"`
	ExpectedStart *float64 `json:"expected_start"`
	ExpectedEnd   *float64 `json:"expected_end"`
	Start         float64  `json:"start"`
	End           float64  `json:"end"`
}

func (s *Service) ReviseMediaJobCue(ctx context.Context, id string, in MediaJobCueRevision) (*MediaProcessingJob, error) {
	if s.mediaJobs == nil {
		return nil, ErrMediaJobsUnavailable
	}
	job, err := s.mediaJobs.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.Status != "review" || job.Operation != "synchronize" {
		return nil, ErrMediaJobConflict
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(job.Result, &result) != nil {
		return nil, ErrMediaJobConflict
	}
	var cues []map[string]json.RawMessage
	if json.Unmarshal(result["cues"], &cues) != nil || in.Paragraph < 1 || in.Paragraph > len(cues) {
		return nil, ErrMediaJobConflict
	}
	var duration float64
	if json.Unmarshal(result["duration_seconds"], &duration) != nil || !finiteTime(in.Start) || !finiteTime(in.End) || in.Start < 0 || in.End <= in.Start || in.End > duration+0.3 {
		return nil, ErrMediaJobConflict
	}
	index := in.Paragraph - 1
	var text string
	if json.Unmarshal(cues[index]["text"], &text) != nil || text != in.Text {
		return nil, ErrMediaJobConflict
	}
	var oldStart, oldEnd *float64
	if raw := cues[index]["start"]; len(raw) > 0 && json.Unmarshal(raw, &oldStart) != nil {
		return nil, ErrMediaJobConflict
	}
	if raw := cues[index]["end"]; len(raw) > 0 && json.Unmarshal(raw, &oldEnd) != nil {
		return nil, ErrMediaJobConflict
	}
	if (oldStart == nil) != (oldEnd == nil) {
		return nil, ErrMediaJobConflict
	}
	if (oldStart == nil) != (in.ExpectedStart == nil) || (oldEnd == nil) != (in.ExpectedEnd == nil) {
		return nil, ErrMediaJobConflict
	}
	if oldStart != nil && (*oldStart != *in.ExpectedStart || *oldEnd != *in.ExpectedEnd) {
		return nil, ErrMediaJobConflict
	}
	before := 0.0
	for i := index - 1; i >= 0; i-- {
		if json.Unmarshal(cues[i]["end"], &before) == nil {
			break
		}
	}
	after := duration
	for i := index + 1; i < len(cues); i++ {
		if json.Unmarshal(cues[i]["start"], &after) == nil {
			break
		}
	}
	if in.Start < before || in.End > after {
		return nil, ErrMediaJobConflict
	}
	cues[index]["start"], _ = json.Marshal(in.Start)
	cues[index]["end"], _ = json.Marshal(in.End)
	delete(cues[index], "timing_status")
	result["cues"], _ = json.Marshal(cues)
	var exceptions []map[string]json.RawMessage
	if json.Unmarshal(result["exceptions"], &exceptions) == nil {
		remaining := make([]map[string]json.RawMessage, 0, len(exceptions))
		for _, entry := range exceptions {
			var paragraph int
			_ = json.Unmarshal(entry["paragraph"], &paragraph)
			if paragraph != in.Paragraph {
				remaining = append(remaining, entry)
			}
		}
		result["exceptions"], _ = json.Marshal(remaining)
	}
	var count int
	_ = json.Unmarshal(result["manual_adjustments"], &count)
	result["manual_adjustments"], _ = json.Marshal(count + 1)
	replacement, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return s.mediaJobs.ReviseResult(ctx, id, job.Result, replacement)
}

func finiteTime(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
