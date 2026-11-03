package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
)

type MediaPublicationReviewInput struct {
	AudioSHA256                  string `json:"audio_sha256"`
	RightsApproved               bool   `json:"rights_approved"`
	RightsReference              string `json:"rights_reference"`
	AudioTranscriptMatchVerified bool   `json:"audio_transcript_match_verified"`
	QualityReview                string `json:"quality_review"`
	Note                         string `json:"note"`
}

// ReviewMediaPublication records an authenticated editorial/rights assessment.
// The action never publishes; publication is separately gated by all checks.
func (s *Service) ReviewMediaPublication(ctx context.Context, itemID string, in MediaPublicationReviewInput) error {
	principal, ok := authn.FromContext(ctx)
	if !ok || !principal.HasRole("admin") || principal.Subject == "" {
		return apperr.New("admin.review_forbidden", apperr.KindForbidden)
	}
	if len(in.AudioSHA256) != 64 || len(in.RightsReference) > 500 || len(in.Note) > 1000 ||
		(in.QualityReview != "required" && in.QualityReview != "approved") ||
		(in.RightsApproved && strings.TrimSpace(in.RightsReference) == "") {
		return apperr.ErrBadRequest
	}
	item, err := s.findMediaItem(ctx, itemID)
	if err != nil {
		return err
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(item.Payload, &payload) != nil {
		return apperr.ErrInternal
	}
	var storedSHA string
	if json.Unmarshal(payload["audio_sha256"], &storedSHA) != nil || !strings.EqualFold(storedSHA, in.AudioSHA256) {
		return ErrMediaJobConflict
	}
	var review map[string]any
	if json.Unmarshal(payload["review"], &review) != nil || review == nil {
		return apperr.ErrBadRequest
	}
	before := map[string]any{"rights_approved": review["rights_approved"],
		"rights_reference": review["rights_reference"], "audio_transcript_match_verified": review["audio_transcript_match_verified"],
		"quality_review": review["quality_review"]}
	review["rights_approved"] = in.RightsApproved
	review["rights_reference"] = strings.TrimSpace(in.RightsReference)
	review["audio_transcript_match_verified"] = in.AudioTranscriptMatchVerified
	review["quality_review"] = in.QualityReview
	payload["review"], _ = json.Marshal(review)
	var history []map[string]any
	if raw := payload["publication_review_history"]; len(raw) > 0 && json.Unmarshal(raw, &history) != nil {
		return ErrMediaJobConflict
	}
	history = append(history, map[string]any{"reviewer_id": principal.Subject,
		"at": time.Now().UTC().Format(time.RFC3339Nano), "note": strings.TrimSpace(in.Note),
		"before": before, "after": map[string]any{"rights_approved": in.RightsApproved,
			"rights_reference":                strings.TrimSpace(in.RightsReference),
			"audio_transcript_match_verified": in.AudioTranscriptMatchVerified, "quality_review": in.QualityReview}})
	payload["publication_review_history"], _ = json.Marshal(history)
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
