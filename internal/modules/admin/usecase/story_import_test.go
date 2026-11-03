package usecase

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

type storyStore struct{ objects map[string][]byte }

func (s *storyStore) Put(_ context.Context, key, _ string, data []byte) (string, error) {
	if s.objects == nil {
		s.objects = map[string][]byte{}
	}
	s.objects[key] = append([]byte(nil), data...)
	return "https://cdn.example/" + key, nil
}

func TestStorySubtitleTimingPreserved(t *testing.T) {
	input := "1\n00:00:01,250 --> 00:00:03,500\nThe boat moved.\n\n2\n00:00:04,000 --> 00:00:06,125\nThe sea was dark.\n"
	cues := transcriptFromFile(input)
	if len(cues) != 2 || cues[0].Start == nil || cues[0].End == nil || *cues[0].Start != 1.25 || *cues[0].End != 3.5 || *cues[1].Start != 4 || *cues[1].End != 6.125 {
		t.Fatalf("subtitle timing lost: %+v", cues)
	}
	plain := transcriptFromFile("The boat moved.\nThe sea was dark.")
	if len(plain) != 2 || plain[0].Start != nil || plain[1].End != nil {
		t.Fatalf("plain transcript should remain untimed: %+v", plain)
	}
	vtt := "WEBVTT\n\nopening\n00:00:01.250 --> 00:00:03.500 align:start\nThe boat moved.\n\nnext\n00:00:04.000 --> 00:00:06.125\nThe sea was dark.\n"
	vttCues := transcriptFromFile(vtt)
	if !subtitleTimingValid(vtt, vttCues) || len(vttCues) != 2 || vttCues[0].Text != "The boat moved." || vttCues[1].Text != "The sea was dark." || *vttCues[0].Start != 1.25 || *vttCues[1].End != 6.125 {
		t.Fatalf("WebVTT cue identifiers/settings broke timing: %+v", vttCues)
	}
	vttLines := subtitleLines(vtt)
	if len(vttLines) != 2 || vttLines[0] != "The boat moved." || vttLines[1] != "The sea was dark." {
		t.Fatalf("WebVTT translation lines included cue identifiers: %+v", vttLines)
	}
	bad := "WEBVTT\n\n00:00:90.000 --> 00:00:95.000\nInvalid clock.\n"
	if subtitleTimingValid(bad, transcriptFromFile(bad)) {
		t.Fatal("malformed timed subtitle was accepted as an untimed transcript")
	}
}

func TestStoryArchiveDryRunStageDuplicateAndPublication(t *testing.T) {
	ctx := context.Background()
	repo := courserepo.NewMemoryCourseRepo()
	store := &storyStore{}
	svc := NewService(repo, nil, store)
	archive, err := os.ReadFile("testdata/leksuu-story-import-test-valid.zip")
	if err != nil {
		t.Fatal(err)
	}

	preview, err := svc.ImportStoryArchive(ctx, archive, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Valid != 1 || preview.Created != 0 || len(store.objects) != 0 {
		t.Fatalf("dry-run mutated state: %+v objects=%d", preview, len(store.objects))
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Items []struct {
			Issues json.RawMessage `json:"issues"`
		} `json:"items"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 1 || string(response.Items[0].Issues) != "[]" {
		t.Fatalf("warning-free preview must encode issues as an array: %s", encoded)
	}

	staged, err := svc.ImportStoryArchive(ctx, archive, false)
	if err != nil {
		t.Fatal(err)
	}
	if staged.Created != 1 || staged.Published {
		t.Fatalf("unexpected staging result: %+v", staged)
	}
	adminItems, err := svc.AdminMedia(ctx, "story")
	if err != nil || len(adminItems) != 1 {
		t.Fatalf("admin stories: %v %+v", err, adminItems)
	}
	var payload map[string]any
	if json.Unmarshal(adminItems[0].Payload, &payload) != nil {
		t.Fatal("payload is not JSON")
	}
	if payload["published"] != false || payload["source_id"] != "test-story-1" {
		t.Fatalf("identity/publication lost: %+v", payload)
	}
	attr, ok := payload["attribution"].(map[string]any)
	if !ok || attr["credit_text"] != "Leksuu QA fixture" {
		t.Fatalf("attribution lost: %+v", payload["attribution"])
	}
	if _, err := svc.UpdateMedia(ctx, adminItems[0].ID, ImportMediaInput{
		Title: "Edited QA Story", Language: "en", CEFR: "A1", URL: payload["url"].(string),
		Transcript: []MediaCue{{Text: "Hello, my friend.", Translation: map[string]string{"fa": "سلام، دوست من.", "ckb": "سڵاو، هاوڕێی من."}}, {Text: "Welcome to Leksuu."}},
	}); err != nil {
		t.Fatal(err)
	}
	adminItems, err = svc.AdminMedia(ctx, "story")
	if err != nil {
		t.Fatal(err)
	}
	payload = map[string]any{}
	_ = json.Unmarshal(adminItems[0].Payload, &payload)
	attr, ok = payload["attribution"].(map[string]any)
	if !ok || attr["credit_text"] != "Leksuu QA fixture" || payload["source_id"] != "test-story-1" {
		t.Fatalf("edit discarded source attribution: %+v", payload)
	}
	if items, total, _ := repo.MediaItems(ctx, "story", "en", 50, 0); total != 0 || len(items) != 0 {
		t.Fatal("draft leaked into learner library")
	}

	again, err := svc.ImportStoryArchive(ctx, archive, false)
	if err != nil || again.Created != 0 || again.Duplicates != 1 {
		t.Fatalf("duplicate protection failed: err=%v report=%+v", err, again)
	}

	if _, err := svc.SetMediaPublished(ctx, adminItems[0].ID, true); err != nil {
		t.Fatal(err)
	}
	items, total, err := repo.MediaItems(ctx, "story", "en", 50, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("published story missing: total=%d err=%v", total, err)
	}
	var published struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(items[0].Payload, &published)
	if published.URL == "" {
		t.Fatal("published story has no playable MP3 URL")
	}
}

func TestPreparedStoryCollectionDryRun(t *testing.T) {
	path := os.Getenv("LEKSUU_STORY_COLLECTION_ZIP")
	if path == "" {
		t.Skip("set LEKSUU_STORY_COLLECTION_ZIP to validate the prepared source collection")
	}
	archive, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil, &storyStore{})
	report, err := svc.ImportStoryArchive(context.Background(), archive, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Total != 20 || report.Valid != 20 || report.Invalid != 0 || report.Created != 0 {
		t.Fatalf("unexpected collection preview: total=%d valid=%d invalid=%d created=%d", report.Total, report.Valid, report.Invalid, report.Created)
	}
}
