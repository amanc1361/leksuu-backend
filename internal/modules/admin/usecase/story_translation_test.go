package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
	"github.com/leksuu/platform/internal/shared/authn"
)

func TestStoryTranslationBackfillPreservesTimingAndRejectsOverwrite(t *testing.T) {
	ctx := context.Background()
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil, &storyStore{})
	archive, err := os.ReadFile("testdata/leksuu-story-import-test-valid.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportStoryArchive(ctx, archive, false); err != nil {
		t.Fatal(err)
	}
	items, err := svc.AdminMedia(ctx, "story")
	if err != nil || len(items) != 1 {
		t.Fatalf("staged story: %v", err)
	}
	item := items[0]
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var transcript []map[string]json.RawMessage
	if err := json.Unmarshal(payload["transcript"], &transcript); err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, cue := range transcript {
		var text string
		_ = json.Unmarshal(cue["text"], &text)
		texts = append(texts, text)
		delete(cue, "translation") // the source fixture has malformed legacy sample text
	}
	start, end := 1.25, 3.5
	transcript[0]["start"], _ = json.Marshal(start)
	transcript[0]["end"], _ = json.Marshal(end)
	transcript[1]["timing_status"], _ = json.Marshal("not_in_audio")
	payload["transcript"], _ = json.Marshal(transcript)
	item.Payload, _ = json.Marshal(payload)
	if err := repo.UpdateItem(ctx, &item); err != nil {
		t.Fatal(err)
	}
	var before map[string]any
	_ = json.Unmarshal(item.Payload, &before)
	var sha string
	_ = json.Unmarshal(payload["audio_sha256"], &sha)
	in := StoryTranslationInput{AudioSHA256: sha, Cues: []StoryTranslationCue{
		{Text: texts[0], Translation: map[string]string{"fa": "سلام، دوست من.", "ckb": "سڵاو، هاوڕێی من."}},
		{Text: texts[1], Translation: map[string]string{"fa": "به لکسو خوش آمدید.", "ckb": "بەخێربێیت بۆ لێکسوو."}},
	}}
	bad := in
	bad.AudioSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := svc.BackfillStoryTranslations(ctx, "test-story-1", bad); !errors.Is(err, ErrStoryTranslationConflict) {
		t.Fatalf("wrong audio accepted: %v", err)
	}
	bad = in
	bad.Cues = append([]StoryTranslationCue(nil), in.Cues...)
	bad.Cues[0].Text = "different source"
	if _, err := svc.BackfillStoryTranslations(ctx, "test-story-1", bad); !errors.Is(err, ErrStoryTranslationConflict) {
		t.Fatalf("changed English accepted: %v", err)
	}
	first, err := svc.BackfillStoryTranslations(ctx, "test-story-1", in)
	if err != nil || !first.Updated || first.Filled["fa"] != 2 || first.Filled["ckb"] != 2 {
		t.Fatalf("translation backfill: %+v %v", first, err)
	}
	retry, err := svc.BackfillStoryTranslations(ctx, "test-story-1", in)
	if err != nil || retry.Updated {
		t.Fatalf("identical retry changed data: %+v %v", retry, err)
	}
	bad = in
	bad.Cues = append([]StoryTranslationCue(nil), in.Cues...)
	bad.Cues[0].Translation = map[string]string{"fa": "متن متفاوت", "ckb": "سڵاو، هاوڕێی من."}
	if _, err := svc.BackfillStoryTranslations(ctx, "test-story-1", bad); !errors.Is(err, ErrStoryTranslationConflict) {
		t.Fatalf("existing translation overwritten: %v", err)
	}
	items, err = svc.AdminMedia(ctx, "story")
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]any
	_ = json.Unmarshal(items[0].Payload, &after)
	if after["translation_status"].(map[string]any)["fa"] != "draft" || after["translation_status"].(map[string]any)["ckb"] != "draft" {
		t.Fatalf("generated translations not marked draft: %+v", after["translation_status"])
	}
	for _, cue := range after["transcript"].([]any) {
		delete(cue.(map[string]any), "translation")
	}
	delete(after, "translation_status")
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("translation backfill changed non-translation data\nbefore=%+v\nafter=%+v", before, after)
	}
	revision := StoryTranslationRevisionInput{AudioSHA256: sha, Paragraph: 1, Text: texts[0], Language: "fa",
		Expected: in.Cues[0].Translation["fa"], Replacement: "سلام، دوست خوب من."}
	changed, err := svc.ReviseStoryTranslation(ctx, "test-story-1", revision)
	if err != nil || !changed.Updated {
		t.Fatalf("draft revision: %+v %v", changed, err)
	}
	if _, err := svc.ReviseStoryTranslation(ctx, "test-story-1", revision); !errors.Is(err, ErrStoryTranslationConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if _, err := svc.ApproveStoryTranslations(ctx, "test-story-1", StoryTranslationApprovalInput{AudioSHA256: sha, Language: "fa"}); !errors.Is(err, ErrStoryTranslationConflict) {
		t.Fatalf("bulk approval skipped cue review: %v", err)
	}
	reviewer := authn.NewContext(ctx, authn.Principal{Subject: "qa-reviewer", Roles: []string{"admin"}})
	for i, expected := range []string{revision.Replacement, in.Cues[1].Translation["fa"]} {
		decision := StoryTranslationDecisionInput{AudioSHA256: sha, Paragraph: i + 1, Text: texts[i], Language: "fa", Expected: expected, Decision: "approved"}
		if _, err := svc.DecideStoryTranslation(reviewer, "test-story-1", decision); err != nil {
			t.Fatalf("cue approval %d: %v", i+1, err)
		}
	}
	if _, err := svc.ReviseStoryTranslation(ctx, "test-story-1", StoryTranslationRevisionInput{AudioSHA256: sha, Paragraph: 1,
		Text: texts[0], Language: "fa", Expected: revision.Replacement, Replacement: "سلام دوباره."}); !errors.Is(err, ErrStoryTranslationConflict) {
		t.Fatalf("approved translation changed: %v", err)
	}
}
