package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
	"github.com/leksuu/platform/internal/shared/authn"
)

const reviewSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestImportedStoryPublicationRequiresActualReview(t *testing.T) {
	ctx := context.Background()
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil)
	lesson, err := svc.ensureMediaLesson(ctx)
	if err != nil {
		t.Fatal(err)
	}
	item, err := svc.AddItem(ctx, lesson, coursedomain.KindStory, json.RawMessage(`{"source_id":"review-qa","audio_sha256":"`+reviewSHA+`","title":"QA","published":false,"review":{"rights_approved":false,"quality_review":"required","audio_transcript_match_verified":false},"translation_status":{"fa":"draft","ckb":"draft"},"transcript":[{"text":"One","start":1,"end":2}]}`), 0, "review-qa")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetMediaPublished(ctx, item.ID, true); !errors.Is(err, ErrMediaPublicationReview) {
		t.Fatalf("unreviewed story published: %v", err)
	}
	result, err := svc.BulkStories(ctx, StoryBulkInput{IDs: []string{item.ID}, Action: "publish"})
	if err != nil || len(result.Succeeded) != 0 || len(result.Failed) != 1 {
		t.Fatalf("bulk publication bypassed review: %+v %v", result, err)
	}
	// A generic item update cannot silently remove review metadata or source cues.
	if _, err := svc.UpdateItem(ctx, item.ID, json.RawMessage(`{"title":"QA","published":true}`), item.MediaKey); err == nil {
		t.Fatal("generic editor bypassed imported story guard")
	}
	input := MediaPublicationReviewInput{AudioSHA256: reviewSHA, RightsApproved: true,
		RightsReference: "local test license", AudioTranscriptMatchVerified: true,
		QualityReview: "approved", Note: "QA fixture only"}
	if err := svc.ReviewMediaPublication(ctx, item.ID, input); err == nil {
		t.Fatal("anonymous publication review was accepted")
	}
	reviewer := authn.NewContext(ctx, authn.Principal{Subject: "qa-reviewer", Roles: []string{"admin"}})
	if err := svc.ReviewMediaPublication(reviewer, item.ID, input); err != nil {
		t.Fatalf("authenticated review: %v", err)
	}
	current, err := svc.findMediaItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	var reviewed struct {
		Published bool             `json:"published"`
		History   []map[string]any `json:"publication_review_history"`
	}
	if err := json.Unmarshal(current.Payload, &reviewed); err != nil {
		t.Fatal(err)
	}
	if reviewed.Published || len(reviewed.History) != 1 {
		t.Fatalf("review published story or lost audit: %+v", reviewed)
	}
	if _, err := svc.SetMediaPublished(ctx, item.ID, true); !errors.Is(err, ErrMediaPublicationReview) {
		t.Fatalf("draft translations bypassed gate: %v", err)
	}
	ready := json.RawMessage(`{"review":{"rights_approved":true,"quality_review":"approved","audio_transcript_match_verified":true},"translation_status":{"fa":"approved","ckb":"approved"},"transcript":[{"text":"One","start":1,"end":2}]}`)
	if !publicationAllowed(ready) {
		t.Fatal("completed review could never pass publication gate")
	}
}

func TestPerCueTranslationApprovalNeedsAdminAndKeepsLanguageDraftUntilComplete(t *testing.T) {
	base := context.Background()
	admin := authn.NewContext(base, authn.Principal{Subject: "reviewer-qa", Roles: []string{"admin"}})
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil)
	lesson, err := svc.ensureMediaLesson(base)
	if err != nil {
		t.Fatal(err)
	}
	item, err := svc.AddItem(base, lesson, coursedomain.KindStory, json.RawMessage(`{"source_id":"translation-review-qa","audio_sha256":"`+reviewSHA+`","published":false,"translation_status":{"fa":"draft"},"transcript":[{"text":"One","translation":{"fa":"یک"}},{"text":"Two","translation":{"fa":"دو"}}]}`), 0, "translation-review-qa")
	if err != nil {
		t.Fatal(err)
	}
	in := StoryTranslationDecisionInput{AudioSHA256: reviewSHA, Paragraph: 1, Text: "One", Language: "fa", Expected: "یک", Decision: "approved"}
	if _, err := svc.DecideStoryTranslation(base, "translation-review-qa", in); err == nil {
		t.Fatal("anonymous review was accepted")
	}
	result, err := svc.DecideStoryTranslation(admin, "translation-review-qa", in)
	if err != nil || !result.Updated {
		t.Fatalf("first review: %+v %v", result, err)
	}
	current, err := svc.findMediaItem(base, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		TranslationStatus map[string]string `json:"translation_status"`
		Transcript        []struct {
			TranslationReview map[string]string `json:"translation_review"`
			History           []map[string]any  `json:"translation_review_history"`
		} `json:"transcript"`
	}
	if err := json.Unmarshal(current.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TranslationStatus["fa"] != "draft" || payload.Transcript[0].TranslationReview["fa"] != "approved" || len(payload.Transcript[0].History) != 1 {
		t.Fatalf("partial review misrepresented: %+v", payload)
	}
	in.Paragraph, in.Text, in.Expected = 2, "Two", "دو"
	if _, err := svc.DecideStoryTranslation(admin, "translation-review-qa", in); err != nil {
		t.Fatal(err)
	}
	current, _ = svc.findMediaItem(base, item.ID)
	if err := json.Unmarshal(current.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TranslationStatus["fa"] != "approved" {
		t.Fatalf("completed review not reflected: %+v", payload.TranslationStatus)
	}
	in.Expected = "stale text"
	if _, err := svc.DecideStoryTranslation(admin, "translation-review-qa", in); !errors.Is(err, ErrStoryTranslationConflict) {
		t.Fatalf("stale decision accepted: %v", err)
	}
}
