package usecase

import (
	"math"
	"strings"

	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

// validateMediaCues applies the same basic timing rules to API submissions as
// the browser importer. Untimed draft lines and a final start-only LRC line
// remain editable; publication has a separate, stricter review gate.
func validateMediaCues(cues []MediaCue) error {
	seen := make(map[string]bool, len(cues))
	previousEnd := 0.0
	for _, cue := range cues {
		if strings.TrimSpace(cue.Text) == "" || len(cue.Text) > 10000 || len(cue.Speaker) > 200 || len(cue.CueID) > 200 ||
			(cue.CueID != "" && seen[cue.CueID]) || len(cue.Words) > 2000 {
			return apperr.ErrBadRequest
		}
		if cue.CueID != "" {
			seen[cue.CueID] = true
		}
		if cue.End != nil && cue.Start == nil {
			return apperr.ErrBadRequest
		}
		if cue.Start != nil {
			if math.IsNaN(*cue.Start) || math.IsInf(*cue.Start, 0) || *cue.Start < previousEnd || *cue.Start > 864000 {
				return apperr.ErrBadRequest
			}
		}
		if cue.End != nil {
			if math.IsNaN(*cue.End) || math.IsInf(*cue.End, 0) || *cue.End <= *cue.Start || *cue.End > 864000 {
				return apperr.ErrBadRequest
			}
			previousEnd = *cue.End
		}
		for lang, value := range cue.Translation {
			if len(lang) > 16 || len(value) > 10000 {
				return apperr.ErrBadRequest
			}
		}
	}
	return nil
}

func ensureMediaCueIDs(cues []MediaCue) {
	for i := range cues {
		if cues[i].CueID == "" {
			cues[i].CueID = id.New()
		}
	}
}
