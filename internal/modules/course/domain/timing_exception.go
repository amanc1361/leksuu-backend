package domain

import (
	"encoding/json"

	"github.com/leksuu/platform/internal/shared/apperr"
)

type TimingExceptionQueueRecord struct {
	SourceID         string
	Paragraph        int
	AudioSHA256      string
	TranscriptSHA256 string
	RecordSHA256     string
	Record           json.RawMessage
}

var ErrTimingExceptionConflict = apperr.New("admin.timing_exception_conflict", apperr.KindConflict)
