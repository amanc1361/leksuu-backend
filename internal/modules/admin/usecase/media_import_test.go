package usecase

import (
	"context"
	"encoding/json"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

func TestImportMediaCreatesLibraryItemInHiddenBucket(t *testing.T) {
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	ctx := context.Background()

	start := 1.5
	it, err := svc.ImportMedia(ctx, ImportMediaInput{
		Kind: "podcast", Title: "Daily News", Language: "en", CEFR: "B1",
		URL:        "https://cdn/x.mp3",
		Transcript: []MediaCue{{Start: &start, Text: "hello", Translation: map[string]string{"fa": "سلام"}}, {Text: "world"}},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if it.Kind != "podcast" {
		t.Fatalf("kind=%s", it.Kind)
	}

	// It appears in the learner media library (transcript present, language matches).
	items, total, err := repo.MediaItems(ctx, "podcast", "en", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("library total=%d len=%d", total, len(items))
	}
	var p struct {
		Title, Language, URL string
		Transcript           []struct {
			Text        string
			Translation map[string]string
		}
	}
	_ = json.Unmarshal(items[0].Payload, &p)
	if p.Title != "Daily News" || p.URL != "https://cdn/x.mp3" || p.Language != "en" || len(p.Transcript) != 2 {
		t.Fatalf("payload mismatch: %+v", p)
	}
	if p.Transcript[0].Translation["fa"] != "سلام" {
		t.Fatalf("translation not persisted: %+v", p.Transcript[0])
	}

	// A second import reuses the same hidden bucket (exactly one is ever created).
	if _, err := svc.ImportMedia(ctx, ImportMediaInput{
		Kind: "video", Title: "Film", URL: "https://cdn/f.mp4", Transcript: []MediaCue{{Text: "a"}},
	}); err != nil {
		t.Fatalf("second import: %v", err)
	}
	all, _ := repo.ListAllCourses(ctx)
	buckets := 0
	for _, c := range all {
		if c.Title == MediaBucketTitle {
			buckets++
		}
	}
	if buckets != 1 {
		t.Fatalf("expected exactly 1 hidden bucket, got %d", buckets)
	}

	// The hidden bucket never leaks into the published learner catalogue.
	pub, _ := repo.ListCourses(ctx, "", "")
	for _, c := range pub {
		if c.Title == MediaBucketTitle {
			t.Fatal("hidden bucket leaked into published course list")
		}
	}

	// Editing in place rewrites the payload (title + transcript) without moving it.
	if _, err := svc.UpdateMedia(ctx, it.ID, ImportMediaInput{
		Kind: "podcast", Title: "Renamed News", Language: "en", CEFR: "B2",
		URL: "https://cdn/y.mp3", Transcript: []MediaCue{{Text: "edited"}},
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	items2, total2, _ := repo.MediaItems(ctx, "podcast", "en", 50, 0)
	if total2 != 1 {
		t.Fatalf("update changed item count: %d", total2)
	}
	var q struct{ Title, URL string }
	for _, m := range items2 {
		if m.ID == it.ID {
			_ = json.Unmarshal(m.Payload, &q)
		}
	}
	if q.Title != "Renamed News" || q.URL != "https://cdn/y.mp3" {
		t.Fatalf("update not applied: %+v", q)
	}
}

func TestImportMediaRejectsBadInput(t *testing.T) {
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil)
	ctx := context.Background()
	cases := []ImportMediaInput{
		{Kind: "text", Title: "x", URL: "u", Transcript: []MediaCue{{Text: "a"}}},   // bad kind
		{Kind: "podcast", Title: "", URL: "u", Transcript: []MediaCue{{Text: "a"}}}, // no title
		{Kind: "podcast", Title: "x", URL: "", Transcript: []MediaCue{{Text: "a"}}}, // no url
		{Kind: "podcast", Title: "x", URL: "u", Transcript: nil},                    // no transcript
	}
	for i, c := range cases {
		if _, err := svc.ImportMedia(ctx, c); err == nil {
			t.Fatalf("case %d: expected error, got nil", i)
		}
	}
}
