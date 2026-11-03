package domain

import (
	"encoding/json"
	"time"

	"github.com/leksuu/platform/internal/shared/apperr"
)

// MediaProcessingJob stores the review state shared by stories, podcasts and films.
type MediaProcessingJob struct {
	ID              string          `json:"id"`
	ItemID          string          `json:"item_id"`
	Kind            string          `json:"kind"`
	Operation       string          `json:"operation"`
	TargetLanguage  string          `json:"target_language,omitempty"`
	PayloadSHA256   string          `json:"payload_sha256"`
	Status          string          `json:"status"`
	Progress        int             `json:"progress"`
	Attempts        int             `json:"attempts"`
	CancelRequested bool            `json:"cancel_requested"`
	Result          json.RawMessage `json:"result,omitempty"`
	ErrorCode       string          `json:"error_code,omitempty"`
	Events          json.RawMessage `json:"events"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

var ErrMediaJobConflict = apperr.New("admin.media_job_conflict", apperr.KindConflict)
