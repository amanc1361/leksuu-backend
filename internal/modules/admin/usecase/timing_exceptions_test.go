package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

func TestTimingExceptionQueueIngestIsGuardedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil)
	svc.SetTimingExceptionStore(NewMemoryTimingExceptionStore())
	lesson, err := svc.ensureMediaLesson(ctx)
	if err != nil {
		t.Fatal(err)
	}
	source := json.RawMessage(`{"source_id":"exception-qa","audio_sha256":"` + reviewSHA + `","published":false,"transcript":[{"text":"First line","timing_status":"unverified"},{"text":"Last line","timing_status":"not_in_audio"}]}`)
	item, err := svc.AddItem(ctx, lesson, coursedomain.KindStory, source, 0, "exception-qa")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := textDigest([]string{"First line", "Last line"})
	if err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf(`{"source_id":"exception-qa","paragraph":1,"source_text":"First line","audio_sha256":"%s","transcript_sha256":"%s","category":"spoken_content_unresolved_timing","status":"editorial_review_required"}`, reviewSHA, hash)
	report := func(value string) json.RawMessage {
		return json.RawMessage(`{"schema":"leksuu.exception-review.v2","records":[` + value + `]}`)
	}
	added, total, err := svc.IngestTimingExceptionReport(ctx, report(record))
	if err != nil || added != 1 || total != 1 {
		t.Fatalf("ingest: %d %d %v", added, total, err)
	}
	added, _, err = svc.IngestTimingExceptionReport(ctx, report(record))
	if err != nil || added != 0 {
		t.Fatalf("retry: %d %v", added, err)
	}
	records, err := svc.TimingExceptionQueue(ctx)
	if err != nil || len(records) != 1 {
		t.Fatalf("queue: %d %v", len(records), err)
	}
	var queued map[string]any
	if err := json.Unmarshal(records[0], &queued); err != nil || queued["status"] != "editorial_review_required" {
		t.Fatalf("record: %v %+v", err, queued)
	}
	changed := fmt.Sprintf(`{"source_id":"exception-qa","paragraph":1,"source_text":"First line","audio_sha256":"%s","transcript_sha256":"%s","category":"other","status":"editorial_review_required"}`, reviewSHA, hash)
	if _, _, err := svc.IngestTimingExceptionReport(ctx, report(changed)); !errors.Is(err, ErrStoryTimingConflict) {
		t.Fatalf("changed evidence accepted: %v", err)
	}
	wrongAudio := fmt.Sprintf(`{"source_id":"exception-qa","paragraph":1,"source_text":"First line","audio_sha256":"%064d","transcript_sha256":"%s","category":"other","status":"editorial_review_required"}`, 0, hash)
	if _, _, err := svc.IngestTimingExceptionReport(ctx, report(wrongAudio)); !errors.Is(err, ErrStoryTimingConflict) {
		t.Fatalf("wrong audio accepted: %v", err)
	}
	current, err := svc.findMediaItem(ctx, item.ID)
	if err != nil || string(current.Payload) != string(source) {
		t.Fatalf("source changed: %v", err)
	}
}
