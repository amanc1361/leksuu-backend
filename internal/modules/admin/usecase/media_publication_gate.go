package usecase

import (
	"encoding/json"
	"path"
	"reflect"
	"strings"

	"github.com/leksuu/platform/internal/shared/apperr"
)

func acceptedMediaReview(value string) bool {
	return value == "approved" || value == "auto_approved"
}

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
		!acceptedMediaReview(item.Review.QualityReview) || !item.Review.AudioTranscriptMatchVerified ||
		!acceptedMediaReview(item.TranslationStatus["fa"]) || !acceptedMediaReview(item.TranslationStatus["ckb"]) ||
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
		return guardPublication(existing, replacement)
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
		Published       bool            `json:"published"`
		Review          json.RawMessage `json:"review"`
		MediaKind       string          `json:"media_kind"`
		RightsApproved  bool            `json:"rights_approved"`
		RightsReference string          `json:"rights_reference"`
		HLSManifestKey  string          `json:"hls_manifest_key"`
		Transcript      []struct {
			CueID           string   `json:"cue_id"`
			Start           *float64 `json:"start"`
			End             *float64 `json:"end"`
			TimingStatus    string   `json:"timing_status"`
			EditorialReview struct {
				Decision string `json:"decision"`
			} `json:"editorial_review"`
			Translation       map[string]string `json:"translation"`
			TranslationReview map[string]string `json:"translation_review"`
			ExerciseReview    string            `json:"exercise_review"`
		} `json:"transcript"`
	}
	if json.Unmarshal(existing, &before) != nil || json.Unmarshal(replacement, &after) != nil {
		return apperr.ErrBadRequest
	}
	if before.Published && after.Published && (before.MediaKind == "music" || before.MediaKind == "video" || before.MediaKind == "podcast") {
		var oldFields, newFields map[string]any
		if json.Unmarshal(existing, &oldFields) != nil || json.Unmarshal(replacement, &newFields) != nil {
			return apperr.ErrBadRequest
		}
		for _, key := range []string{"url", "transcript", "rights_approved", "rights_reference", "hls_manifest_key", "translation_status", "review"} {
			if !reflect.DeepEqual(oldFields[key], newFields[key]) {
				return ErrMediaPublicationReview
			}
		}
	}
	if !before.Published && after.Published && (before.MediaKind == "music" || before.MediaKind == "video" || before.MediaKind == "podcast") {
		if !before.RightsApproved || strings.TrimSpace(before.RightsReference) == "" || len(before.Transcript) == 0 {
			return ErrMediaPublicationReview
		}
		if (before.MediaKind == "music" || before.MediaKind == "video") &&
			(!strings.HasPrefix(before.HLSManifestKey, "hls/") || path.Base(before.HLSManifestKey) != "master.m3u8") {
			return ErrMediaPublicationReview
		}
		for _, cue := range before.Transcript {
			if cue.CueID == "" || cue.Start == nil || cue.End == nil || *cue.Start < 0 || *cue.End <= *cue.Start ||
				!acceptedMediaReview(cue.TimingStatus) || !acceptedMediaReview(cue.EditorialReview.Decision) ||
				!acceptedMediaReview(cue.TranslationReview["fa"]) ||
				!acceptedMediaReview(cue.TranslationReview["ckb"]) || cue.Translation["fa"] == "" || cue.Translation["ckb"] == "" {
				return ErrMediaPublicationReview
			}
			if before.MediaKind != "podcast" && !acceptedMediaReview(cue.ExerciseReview) {
				return ErrMediaPublicationReview
			}
		}
	}
	if !before.Published && after.Published && len(before.Review) > 0 && string(before.Review) != "null" && !publicationAllowed(existing) {
		return ErrMediaPublicationReview
	}
	return nil
}
