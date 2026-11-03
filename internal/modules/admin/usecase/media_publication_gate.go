package usecase

import (
	"encoding/json"
	"reflect"

	"github.com/leksuu/platform/internal/shared/apperr"
)

var ErrMediaPublicationReview = apperr.New("admin.media_publication_review_required", apperr.KindConflict)

// publicationAllowed is deliberately conservative for imported stories with
// review metadata. A generated draft or incomplete timing never satisfies it.
func publicationAllowed(payload json.RawMessage) bool {
	var item struct {
		Review struct {
			RightsApproved               bool   `json:"rights_approved"`
			QualityReview                string `json:"quality_review"`
			AudioTranscriptMatchVerified bool   `json:"audio_transcript_match_verified"`
		} `json:"review"`
		TranslationStatus map[string]string `json:"translation_status"`
		Transcript        []struct {
			Start           *float64 `json:"start"`
			End             *float64 `json:"end"`
			TimingStatus    string   `json:"timing_status"`
			EditorialReview struct {
				Decision string `json:"decision"`
			} `json:"editorial_review"`
		} `json:"transcript"`
	}
	if json.Unmarshal(payload, &item) != nil || !item.Review.RightsApproved ||
		item.Review.QualityReview != "approved" || !item.Review.AudioTranscriptMatchVerified ||
		item.TranslationStatus["fa"] != "approved" || item.TranslationStatus["ckb"] != "approved" ||
		len(item.Transcript) == 0 {
		return false
	}
	for _, cue := range item.Transcript {
		if cue.Start != nil && cue.End != nil && *cue.Start >= 0 && *cue.End > *cue.Start {
			continue
		}
		if cue.TimingStatus != "not_in_audio" || cue.EditorialReview.Decision != "not_in_audio" {
			return false
		}
	}
	return true
}

func guardImportedMutation(existing, replacement json.RawMessage) error {
	var before, after map[string]any
	if json.Unmarshal(existing, &before) != nil || json.Unmarshal(replacement, &after) != nil {
		return apperr.ErrBadRequest
	}
	if before["source_id"] == nil {
		return nil
	}
	for _, key := range []string{"source_id", "source_url", "audio_sha256", "url", "transcript", "translation_status", "review"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			return ErrStoryTimingConflict
		}
	}
	return guardPublication(existing, replacement)
}

func guardPublication(existing, replacement json.RawMessage) error {
	var before, after struct {
		Published bool            `json:"published"`
		Review    json.RawMessage `json:"review"`
	}
	if json.Unmarshal(existing, &before) != nil || json.Unmarshal(replacement, &after) != nil {
		return apperr.ErrBadRequest
	}
	if !before.Published && after.Published && len(before.Review) > 0 && string(before.Review) != "null" && !publicationAllowed(existing) {
		return ErrMediaPublicationReview
	}
	return nil
}
