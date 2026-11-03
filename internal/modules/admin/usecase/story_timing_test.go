package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

func TestStoryTimingBackfillPreservesPayloadAndRejectsMismatches(t *testing.T) {
	point := func(value float64) *float64 { return &value }
	ctx := context.Background()
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil, &storyStore{})
	archive, err := os.ReadFile("testdata/leksuu-story-import-test-valid.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportStoryArchive(ctx, archive, false); err != nil {
		t.Fatal(err)
	}
	items, err := svc.AdminMedia(ctx, "story")
	if err != nil || len(items) != 1 {
		t.Fatalf("staged story: %v %+v", err, items)
	}
	var original map[string]any
	if err := json.Unmarshal(items[0].Payload, &original); err != nil {
		t.Fatal(err)
	}
	cues := original["transcript"].([]any)
	in := StoryTimingInput{AudioSHA256: original["audio_sha256"].(string), Cues: []StoryTimingCue{
		{Text: cues[0].(map[string]any)["text"].(string), Start: point(1.25), End: point(3.5)},
		{Text: cues[1].(map[string]any)["text"].(string), TimingStatus: "not_in_audio"},
	}}
	badSHA := in
	badSHA.AudioSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := svc.BackfillStoryTiming(ctx, "test-story-1", badSHA); !errors.Is(err, ErrStoryTimingConflict) {
		t.Fatalf("wrong audio digest was accepted: %v", err)
	}
	badText := in
	badText.Cues = append([]StoryTimingCue(nil), in.Cues...)
	badText.Cues[0].Text = "changed text"
	if _, err := svc.BackfillStoryTiming(ctx, "test-story-1", badText); !errors.Is(err, ErrStoryTimingConflict) {
		t.Fatalf("changed transcript was accepted: %v", err)
	}
	badTiming := in
	badTiming.Cues = append([]StoryTimingCue(nil), in.Cues...)
	badTiming.Cues[1].Start = point(3)
	badTiming.Cues[1].End = point(4)
	badTiming.Cues[1].TimingStatus = ""
	if _, err := svc.BackfillStoryTiming(ctx, "test-story-1", badTiming); err == nil {
		t.Fatal("overlapping times were accepted")
	}
	result, err := svc.BackfillStoryTiming(ctx, "test-story-1", in)
	if err != nil || !result.Updated || result.TimedCues != 1 || result.UntimedCues != 1 || result.ID != items[0].ID {
		t.Fatalf("timing backfill failed: %+v %v", result, err)
	}
	items, err = svc.AdminMedia(ctx, "story")
	if err != nil {
		t.Fatal(err)
	}
	var updated map[string]any
	if err := json.Unmarshal(items[0].Payload, &updated); err != nil {
		t.Fatal(err)
	}
	updatedCues := updated["transcript"].([]any)
	if updatedCues[0].(map[string]any)["start"] != 1.25 || updatedCues[1].(map[string]any)["timing_status"] != "not_in_audio" {
		t.Fatalf("timing not saved: %+v", updatedCues)
	}
	for i := range cues {
		delete(updatedCues[i].(map[string]any), "start")
		delete(updatedCues[i].(map[string]any), "end")
		delete(updatedCues[i].(map[string]any), "timing_status")
	}
	if !reflect.DeepEqual(original, updated) {
		t.Fatalf("backfill changed non-timing fields\noriginal=%+v\nupdated=%+v", original, updated)
	}
	retry, err := svc.BackfillStoryTiming(ctx, "test-story-1", in)
	if err != nil || retry.Updated {
		t.Fatalf("identical retry should be a no-op: %+v %v", retry, err)
	}
	changed := in
	changed.Cues = append([]StoryTimingCue(nil), in.Cues...)
	changed.Cues[0].Start = point(1.5)
	if _, err := svc.BackfillStoryTiming(ctx, "test-story-1", changed); !errors.Is(err, ErrStoryTimingConflict) {
		t.Fatalf("changed timing was accepted: %v", err)
	}
}

func TestStoryTimingRevisionUpdatesOneCueSafely(t *testing.T) {
	point := func(value float64) *float64 { return &value }
	ctx := context.Background()
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil, &storyStore{})
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
	var payload map[string]any
	_ = json.Unmarshal(items[0].Payload, &payload)
	cues := payload["transcript"].([]any)
	first := cues[0].(map[string]any)["text"].(string)
	second := cues[1].(map[string]any)["text"].(string)
	sha := payload["audio_sha256"].(string)
	initial := StoryTimingInput{AudioSHA256: sha, Cues: []StoryTimingCue{
		{Text: first, Start: point(1), End: point(3)},
		{Text: second, TimingStatus: "unverified"},
	}}
	if _, err := svc.BackfillStoryTiming(ctx, "test-story-1", initial); err != nil {
		t.Fatal(err)
	}
	in := StoryTimingRevisionInput{AudioSHA256: sha, Paragraph: 2,
		Expected:    StoryTimingCue{Text: second, TimingStatus: "unverified"},
		Replacement: StoryTimingCue{Text: second, Start: point(4), End: point(6)}}
	bad := in
	bad.Replacement = StoryTimingCue{Text: second, Start: point(2), End: point(5)}
	if _, err := svc.ReviseStoryTiming(ctx, "test-story-1", bad); err == nil {
		t.Fatal("overlapping revision accepted")
	}
	bad = in
	bad.AudioSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := svc.ReviseStoryTiming(ctx, "test-story-1", bad); !errors.Is(err, ErrStoryTimingConflict) {
		t.Fatalf("wrong audio accepted: %v", err)
	}
	result, err := svc.ReviseStoryTiming(ctx, "test-story-1", in)
	if err != nil || !result.Updated {
		t.Fatalf("revision failed: %+v %v", result, err)
	}
	retry, err := svc.ReviseStoryTiming(ctx, "test-story-1", in)
	if err != nil || retry.Updated {
		t.Fatalf("identical revision retry changed data: %+v %v", retry, err)
	}
	items, err = svc.AdminMedia(ctx, "story")
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]any
	_ = json.Unmarshal(items[0].Payload, &after)
	updated := after["transcript"].([]any)
	if updated[0].(map[string]any)["start"] != float64(1) || updated[1].(map[string]any)["start"] != float64(4) || after["published"] != false {
		t.Fatalf("revision altered other fields: %+v", after)
	}
	locked := StoryTimingRevisionInput{AudioSHA256: sha, Paragraph: 2,
		Expected:    StoryTimingCue{Text: second, Start: point(4), End: point(6)},
		Replacement: StoryTimingCue{Text: second, TimingStatus: "not_in_audio"}}
	if _, err := svc.ReviseStoryTiming(ctx, "test-story-1", locked); err != nil {
		t.Fatalf("editorial not-in-audio classification failed: %v", err)
	}
	locked.Expected = locked.Replacement
	locked.Replacement = StoryTimingCue{Text: second, Start: point(4), End: point(6)}
	if _, err := svc.ReviseStoryTiming(ctx, "test-story-1", locked); !errors.Is(err, ErrStoryTimingConflict) {
		t.Fatalf("confirmed not-in-audio cue changed: %v", err)
	}
}
