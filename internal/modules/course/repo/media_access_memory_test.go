package repo

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/course/domain"
)

func TestPlaybackLookupSupportsAllLearnerMediaProgressKinds(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryCourseRepo()
	for _, kind := range []string{domain.KindStory, domain.KindPodcast, domain.KindVideo, domain.KindMusic} {
		item := domain.LessonItem{LessonID: "lesson-greetings", Kind: kind, Payload: []byte(`{"language":"en","published":true}`)}
		if err := repo.CreateItem(ctx, &item); err != nil {
			t.Fatal(err)
		}
		got, err := repo.PlaybackItem(ctx, item.ID)
		if err != nil || got.Kind != kind {
			t.Fatalf("%s lookup: %+v %v", kind, got, err)
		}
	}
	text := domain.LessonItem{LessonID: "lesson-greetings", Kind: domain.KindText}
	if err := repo.CreateItem(ctx, &text); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PlaybackItem(ctx, text.ID); err != domain.ErrItemNotFound {
		t.Fatalf("text accepted as media: %v", err)
	}
}
