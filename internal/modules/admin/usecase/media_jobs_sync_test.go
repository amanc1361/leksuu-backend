package usecase

import (
	"context"
	"errors"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

func TestEnqueueMediaJobSynchronizesParagraphTimingWithoutWords(t *testing.T) {
	ctx := context.Background()
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil)
	jobs := &recordingJobStore{}
	svc.SetMediaJobs(jobs)
	start, end := 1.0, 2.0
	item, err := svc.ImportMedia(ctx, ImportMediaInput{
		Kind: "story", Title: "Timed story", Language: "en", URL: "https://leksuu.ir/media/story.mp3",
		Transcript: []MediaCue{{Text: "Two words.", Start: &start, End: &end}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnqueueMediaJob(ctx, item.ID); err != nil {
		t.Fatalf("paragraph-only timing should be synchronized: %v", err)
	}
	if jobs.enqueued != 1 {
		t.Fatalf("enqueued=%d", jobs.enqueued)
	}
}

func TestEnqueueMediaJobSkipsExactWordTiming(t *testing.T) {
	ctx := context.Background()
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil)
	jobs := &recordingJobStore{}
	svc.SetMediaJobs(jobs)
	start, end := 1.0, 2.0
	item, err := svc.ImportMedia(ctx, ImportMediaInput{
		Kind: "story", Title: "Exactly timed story", Language: "en", URL: "https://leksuu.ir/media/story.mp3",
		Transcript: []MediaCue{{Text: "Two words.", Start: &start, End: &end, Words: []MediaWordTiming{
			{Word: "Two", Start: 1.0, End: 1.35}, {Word: "words", Start: 1.4, End: 1.9},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EnqueueMediaJob(ctx, item.ID); !errors.Is(err, ErrMediaJobConflict) {
		t.Fatalf("exact timing should not be queued again: %v", err)
	}
	if jobs.enqueued != 0 {
		t.Fatalf("enqueued=%d", jobs.enqueued)
	}
}
