package usecase

import (
	"context"
	"encoding/json"
	"errors"
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
		Transcript: []MediaCue{{Start: &start, Text: "hello", Speaker: "Host", Translation: map[string]string{"fa": "سلام"}}, {Text: "world"}},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if it.Kind != "podcast" {
		t.Fatalf("kind=%s", it.Kind)
	}

	// New uploads are drafts until an administrator explicitly publishes them.
	items, total, err := repo.MediaItems(ctx, "podcast", "en", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("draft leaked into library total=%d len=%d", total, len(items))
	}
	if _, err := svc.SetMediaPublished(ctx, it.ID, true); err == nil {
		t.Fatal("unreviewed podcast became visible")
	}
	var reviewed map[string]any
	if err := json.Unmarshal(it.Payload, &reviewed); err != nil {
		t.Fatal(err)
	}
	reviewed["rights_approved"] = true
	reviewed["rights_reference"] = "disposable test recording"
	for index, raw := range reviewed["transcript"].([]any) {
		cue := raw.(map[string]any)
		cue["start"] = float64(index * 2)
		cue["end"] = float64(index*2 + 1)
		cue["timing_status"] = "approved"
		cue["editorial_review"] = map[string]any{"decision": "approved"}
		translations, _ := cue["translation"].(map[string]any)
		if translations == nil {
			translations = map[string]any{}
		}
		if translations["fa"] == nil {
			translations["fa"] = "test-fa"
		}
		translations["ckb"] = "test-ckb"
		cue["translation"] = translations
		cue["translation_review"] = map[string]any{"fa": "approved", "ckb": "approved"}
	}
	it.Payload, err = json.Marshal(reviewed)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetMediaPublished(ctx, it.ID, true); err != nil {
		t.Fatal(err)
	}
	items, total, err = repo.MediaItems(ctx, "podcast", "en", 50, 0)
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("published item missing: %v %d %d", err, total, len(items))
	}
	var p struct {
		Title, Language, URL string
		Transcript           []struct {
			Text        string
			Speaker     string
			Translation map[string]string
		}
	}
	_ = json.Unmarshal(items[0].Payload, &p)
	if p.Title != "Daily News" || p.URL != "https://cdn/x.mp3" || p.Language != "en" || len(p.Transcript) != 2 || p.Transcript[0].Speaker != "Host" {
		t.Fatalf("payload mismatch: %+v", p)
	}
	if p.Transcript[0].Translation["fa"] != "سلام" {
		t.Fatalf("translation not persisted: %+v", p.Transcript[0])
	}

	// A second import reuses the same hidden bucket (exactly one is ever created).
	second, err := svc.ImportMedia(ctx, ImportMediaInput{
		Kind: "video", Title: "Film", URL: "https://cdn/f.mp4", Transcript: []MediaCue{{Text: "a"}},
	})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.Position <= it.Position {
		t.Fatalf("second import must append: first position=%d second=%d", it.Position, second.Position)
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

	// A published recording must be withdrawn before replacing its reviewed cues.
	if _, err := svc.UpdateMedia(ctx, it.ID, ImportMediaInput{Kind: "podcast", Title: "Changed while public", URL: "https://cdn/y.mp3", Transcript: []MediaCue{{Text: "edited"}}}); !errors.Is(err, ErrMediaPublicationReview) {
		t.Fatalf("published source changed without review: %v", err)
	}
	if _, err := svc.SetMediaPublished(ctx, it.ID, false); err != nil {
		t.Fatal(err)
	}
	// Editing in place rewrites the draft payload without moving it.
	if _, err := svc.UpdateMedia(ctx, it.ID, ImportMediaInput{
		Kind: "podcast", Title: "Renamed News", Language: "en", CEFR: "B2",
		URL: "https://cdn/y.mp3", Transcript: []MediaCue{{Text: "edited"}},
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	items2, err := svc.AdminMedia(ctx, "podcast")
	if err != nil || len(items2) != 1 {
		t.Fatalf("updated draft missing: %v, count %d", err, len(items2))
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

func TestMusicImportStaysDraftAndKeepsCueIdentity(t *testing.T) {
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	start, end := 1.0, 2.0
	item, err := svc.ImportMedia(context.Background(), ImportMediaInput{
		Kind: "music", Title: "Authorized recording", Artist: "Example artist", Language: "de", AudioLanguage: "de",
		URL:        "https://example.invalid/song.mp3",
		Transcript: []MediaCue{{CueID: "source-cue-1", Start: &start, End: &end, Text: "Guten Morgen", TimingStatus: "verified"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Kind != "music" {
		t.Fatalf("kind=%s", item.Kind)
	}
	var payload struct {
		Artist     string     `json:"artist"`
		Transcript []MediaCue `json:"transcript"`
		Published  bool       `json:"published"`
	}
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Published || payload.Artist != "Example artist" || payload.Transcript[0].CueID != "source-cue-1" {
		t.Fatalf("music draft or cue identity lost: %+v", payload)
	}
	_, total, err := repo.MediaItems(context.Background(), "music", "de", 10, 0)
	if err != nil || total != 0 {
		t.Fatalf("draft leaked: %v, %d", err, total)
	}
	if _, err := svc.SetMediaPublished(context.Background(), item.ID, true); err == nil {
		t.Fatal("music without rights review was published")
	}
}

func TestMusicPublicationRequiresReviewedRightsAndTiming(t *testing.T) {
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	start, end := 1.0, 2.0
	base := ImportMediaInput{Kind: "music", Title: "Song", Language: "de", URL: "https://example.invalid/song.mp3",
		RightsApproved: true, RightsReference: "licensed-fixture", Transcript: []MediaCue{{CueID: "one", Start: &start, End: &end, Text: "Guten Morgen", TimingStatus: "verified"}}}
	item, err := svc.ImportMedia(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetMediaPublished(context.Background(), item.ID, true); err == nil {
		t.Fatal("music without HLS and cue approvals was published")
	}
	var reviewed map[string]any
	if err := json.Unmarshal(item.Payload, &reviewed); err != nil {
		t.Fatal(err)
	}
	reviewed["hls_manifest_key"] = "hls/fixture/master.m3u8"
	cues := reviewed["transcript"].([]any)
	cue := cues[0].(map[string]any)
	cue["timing_status"] = "approved"
	cue["editorial_review"] = map[string]any{"decision": "approved"}
	cue["translation"] = map[string]any{"fa": "صبح بخیر", "ckb": "بەیانی باش"}
	cue["translation_review"] = map[string]any{"fa": "approved", "ckb": "approved"}
	cue["exercise_review"] = "approved"
	item.Payload, err = json.Marshal(reviewed)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateItem(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetMediaPublished(context.Background(), item.ID, true); err != nil {
		t.Fatal(err)
	}
	base.Transcript[0].TimingStatus = "draft"
	item, err = svc.ImportMedia(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetMediaPublished(context.Background(), item.ID, true); err == nil {
		t.Fatal("unverified lyric timing was published")
	}
}

func TestNewFilmPublicationRequiresRightsReview(t *testing.T) {
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	start, end := 0.0, 2.0
	item, err := svc.ImportMedia(context.Background(), ImportMediaInput{Kind: "video", Title: "Film", Language: "de", URL: "https://example.invalid/film.mp4",
		Transcript: []MediaCue{{CueID: "film-cue", Start: &start, End: &end, Text: "Guten Tag", TimingStatus: "verified"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetMediaPublished(context.Background(), item.ID, true); err == nil {
		t.Fatal("film published without rights")
	}
}

func TestImportMediaRejectsBadInput(t *testing.T) {
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil)
	ctx := context.Background()
	cases := []ImportMediaInput{
		{Kind: "text", Title: "x", URL: "u", Transcript: []MediaCue{{Text: "a"}}},   // bad kind
		{Kind: "podcast", Title: "", URL: "u", Transcript: []MediaCue{{Text: "a"}}}, // no title
		{Kind: "podcast", Title: "x", URL: "", Transcript: []MediaCue{{Text: "a"}}}, // no url
		{Kind: "story", Title: "x", URL: "", Transcript: nil},                       // no audio or transcript
		{Kind: "podcast", Title: "x", URL: "u", Transcript: []MediaCue{{CueID: "same", Text: "a"}, {CueID: "same", Text: "b"}}},
	}
	for i, c := range cases {
		if _, err := svc.ImportMedia(ctx, c); err == nil {
			t.Fatalf("case %d: expected error, got nil", i)
		}
	}
}
