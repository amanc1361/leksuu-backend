package usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

type recordingJobStore struct {
	enqueued int
	snapshot json.RawMessage
}

func (s *recordingJobStore) Enqueue(_ context.Context, itemID, kind, operation, targetLanguage, payloadSHA256 string, payload json.RawMessage) (*MediaProcessingJob, error) {
	s.enqueued++
	s.snapshot = append([]byte(nil), payload...)
	return &MediaProcessingJob{ID: "queued-job", ItemID: itemID, Kind: kind, Operation: operation, PayloadSHA256: payloadSHA256, Status: "pending"}, nil
}
func (*recordingJobStore) List(context.Context, int) ([]MediaProcessingJob, error)  { return nil, nil }
func (*recordingJobStore) Get(context.Context, string) (*MediaProcessingJob, error) { return nil, nil }
func (*recordingJobStore) Cancel(context.Context, string) (*MediaProcessingJob, error) {
	return nil, nil
}
func (*recordingJobStore) Retry(context.Context, string) (*MediaProcessingJob, error) {
	return nil, nil
}
func (*recordingJobStore) Approve(context.Context, string, string) (*MediaProcessingJob, error) {
	return nil, nil
}
func (*recordingJobStore) ReviseResult(context.Context, string, json.RawMessage, json.RawMessage) (*MediaProcessingJob, error) {
	return nil, nil
}

func metadataStoryZIP(t *testing.T, cues []MediaCue) []byte {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	add := func(name string, body []byte) {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	add("manifest.json", []byte(`{"schema":"leksuu.story-import.v1","items":[{"source_id":"metadata-test","path":"items/metadata-test.json"}]}`))
	item, err := json.Marshal(map[string]any{"kind": "story", "source_id": "metadata-test", "title": "Metadata test", "language": "en", "audio": "media/metadata-test.mp3", "transcript": cues})
	if err != nil {
		t.Fatal(err)
	}
	add("items/metadata-test.json", item)
	add("media/metadata-test.mp3", []byte("ID3test data"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestStoryImportPreservesMetadataAndPartialTiming(t *testing.T) {
	start, end := 2.25, 3.75
	cues := []MediaCue{
		{CueID: "intro", ContentKind: "introduction", Text: "Spoken introduction.", Start: &start, End: &end,
			Words:       []MediaWordTiming{{Start: 2.25, End: 2.7, Word: "Spoken"}, {Start: 2.8, End: 3.75, Word: "introduction"}},
			Translation: map[string]string{"fa": "مقدمه", "ckb": "پێشەکی"}},
		{CueID: "story", ContentKind: "narrative", Text: "A recap remains readable.", TimingStatus: "unverified",
			Translation: map[string]string{"fa": "خلاصه", "ckb": "پوختە"}},
	}
	items, _, err := parseStoryArchive(metadataStoryZIP(t, cues))
	if err != nil || len(items) != 1 || !items[0].Valid || items[0].TimedCues != 1 {
		t.Fatalf("partial timing rejected: %+v %v", items, err)
	}
	got := items[0].item.Transcript
	if got[0].CueID != "intro" || got[0].ContentKind != "introduction" || got[0].Words[0].Start != 2.25 ||
		got[0].Translation["ckb"] != "پێشەکی" || got[1].TimingStatus != "unverified" || got[1].ContentKind != "narrative" {
		t.Fatalf("metadata lost: %+v", got)
	}
}

func TestStoryImportRejectsContradictoryAndDuplicateCueMetadata(t *testing.T) {
	start, end := 1.0, 2.0
	cues := []MediaCue{{CueID: "same", ContentKind: "introduction", Text: "First.", Start: &start, End: &end, TimingStatus: "not_in_audio"},
		{CueID: "same", ContentKind: "invalid", Text: "Second."}}
	items, _, err := parseStoryArchive(metadataStoryZIP(t, cues))
	if err != nil || len(items) != 1 || items[0].Valid {
		t.Fatalf("bad cues accepted: %+v %v", items, err)
	}
	codes := map[string]bool{}
	for _, issue := range items[0].Issues {
		codes[issue.Code] = true
	}
	for _, code := range []string{"cue_id_duplicate", "content_kind_invalid", "timing_contradiction"} {
		if !codes[code] {
			t.Errorf("missing %s in %+v", code, items[0].Issues)
		}
	}
}

func TestStoryImportQueuesOnlyNewDraftsWithOriginalPayload(t *testing.T) {
	ctx := context.Background()
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil, &storyStore{})
	jobs := &recordingJobStore{}
	svc.SetMediaJobs(jobs)
	archive := metadataStoryZIP(t, []MediaCue{{Text: "Introduction.", ContentKind: "introduction"}, {Text: "Narrative."}})
	preview, err := svc.ImportStoryArchive(ctx, archive, true)
	if err != nil || preview.Created != 0 || jobs.enqueued != 0 {
		t.Fatalf("dry run queued work: %+v %v", preview, err)
	}
	staged, err := svc.ImportStoryArchive(ctx, archive, false)
	if err != nil || staged.Created != 1 || jobs.enqueued != 1 || staged.Items[0].ProcessingJobID != "queued-job" {
		t.Fatalf("new draft was not queued: %+v %v", staged, err)
	}
	var payload map[string]any
	if json.Unmarshal(jobs.snapshot, &payload) != nil || payload["published"] != false || payload["audio_sha256"] == "" {
		t.Fatalf("job snapshot lost draft or audio identity: %+v", payload)
	}
	again, err := svc.ImportStoryArchive(ctx, archive, false)
	if err != nil || again.Duplicates != 1 || jobs.enqueued != 1 {
		t.Fatalf("repeat import queued again: %+v %v", again, err)
	}
}
