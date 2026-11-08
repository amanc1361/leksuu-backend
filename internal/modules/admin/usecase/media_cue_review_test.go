package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
	"github.com/leksuu/platform/internal/shared/authn"
)

func TestMediaCueReviewPreservesApprovedDataAndRejectsStaleEdits(t *testing.T) {
	ctx := authn.NewContext(context.Background(), authn.Principal{Subject: "reviewer", Roles: []string{"admin"}})
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	start, end := 1.0, 3.0
	item, err := svc.ImportMedia(ctx, ImportMediaInput{Kind: "video", Title: "Fixture", URL: "https://example.invalid/fixture.mp4",
		Transcript: []MediaCue{{CueID: "cue-1", Start: &start, End: &end, Text: "Hello there",
			Translation: map[string]string{"fa": "سلام", "ckb": "سڵاو"}}}})
	if err != nil {
		t.Fatal(err)
	}
	for revision, action := range []string{"source_approve", "fa_approve", "ckb_approve", "exercise_approve"} {
		if err := svc.ReviewMediaCue(ctx, item.ID, MediaCueReviewInput{CueID: "cue-1", ExpectedRevision: revision, Action: action}); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if err := svc.ReviewMediaCue(ctx, item.ID, MediaCueReviewInput{CueID: "cue-1", ExpectedRevision: 0, Action: "source_reopen"}); !errors.Is(err, ErrMediaJobConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if err := svc.ReviewMediaCue(ctx, item.ID, MediaCueReviewInput{CueID: "cue-1", ExpectedRevision: 4, Action: "edit",
		Text: "Changed there", Start: &start, End: &end}); !errors.Is(err, ErrMediaPublicationReview) {
		t.Fatalf("approved source changed without reopening: %v", err)
	}
	if err := svc.ReviewMediaCue(ctx, item.ID, MediaCueReviewInput{CueID: "cue-1", ExpectedRevision: 4, Action: "source_reopen"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ReviewMediaCue(ctx, item.ID, MediaCueReviewInput{CueID: "cue-1", ExpectedRevision: 5, Action: "edit",
		Text: "Changed there", Start: &start, End: &end}); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.findMediaItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Transcript []struct {
			Text              string            `json:"text"`
			Translation       map[string]string `json:"translation"`
			TranslationReview map[string]string `json:"translation_review"`
			ExerciseReview    string            `json:"exercise_review"`
			ReviewHistory     []any             `json:"review_history"`
		} `json:"transcript"`
	}
	if json.Unmarshal(updated.Payload, &payload) != nil || len(payload.Transcript) != 1 {
		t.Fatal("reviewed payload unavailable")
	}
	cue := payload.Transcript[0]
	if cue.Text != "Changed there" || cue.Translation["fa"] != "سلام" || cue.Translation["ckb"] != "سڵاو" ||
		cue.TranslationReview["fa"] != "draft" || cue.TranslationReview["ckb"] != "draft" ||
		cue.ExerciseReview != "draft" || len(cue.ReviewHistory) != 6 {
		t.Fatalf("review preservation failed: %+v", cue)
	}
}
