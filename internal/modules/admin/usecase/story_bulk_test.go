package usecase

import (
	"context"
	"encoding/json"
	"testing"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

func TestBulkStoriesValidatesAndUpdatesOnlySelectedStories(t *testing.T) {
	ctx := context.Background()
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil)
	lessonID, err := svc.ensureMediaLesson(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.AddItem(ctx, lessonID, coursedomain.KindStory, json.RawMessage(`{"title":"A Part One","published":false,"transcript":[{"text":"One"}]}`), 0, "story-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.AddItem(ctx, lessonID, coursedomain.KindStory, json.RawMessage(`{"title":"A Part Two","published":false,"transcript":[{"text":"Two"}]}`), 1, "story-2")
	if err != nil {
		t.Fatal(err)
	}
	film, err := svc.AddItem(ctx, lessonID, coursedomain.KindVideo, json.RawMessage(`{"title":"Film"}`), 2, "film-1")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.BulkStories(ctx, StoryBulkInput{IDs: []string{first.ID, film.ID}, Action: "delete"}); err == nil {
		t.Fatal("mixed story and film selection should be rejected")
	}
	if _, err := svc.BulkStories(ctx, StoryBulkInput{IDs: []string{first.ID, first.ID}, Action: "delete"}); err == nil {
		t.Fatal("duplicate IDs should be rejected")
	}
	items, err := svc.AdminMedia(ctx, "")
	if err != nil || len(items) != 3 {
		t.Fatalf("validation changed items: %v, %d", err, len(items))
	}

	level := "B1"
	result, err := svc.BulkStories(ctx, StoryBulkInput{IDs: []string{first.ID, second.ID}, Action: "edit", Level: &level})
	if err != nil || len(result.Succeeded) != 2 || len(result.Failed) != 0 {
		t.Fatalf("bulk edit: %+v %v", result, err)
	}
	result, err = svc.BulkStories(ctx, StoryBulkInput{IDs: []string{first.ID, second.ID}, Action: "publish"})
	if err != nil || len(result.Succeeded) != 2 {
		t.Fatalf("bulk publication: %+v %v", result, err)
	}
	stories, err := svc.AdminMedia(ctx, coursedomain.KindStory)
	if err != nil || len(stories) != 2 {
		t.Fatalf("updated stories: %v, %d", err, len(stories))
	}
	for _, story := range stories {
		var payload map[string]any
		if err := json.Unmarshal(story.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["level"] != "B1" || payload["published"] != true || payload["transcript"] == nil {
			t.Fatalf("bulk update lost payload: %+v", payload)
		}
	}
	result, err = svc.BulkStories(ctx, StoryBulkInput{IDs: []string{second.ID}, Action: "unpublish"})
	if err != nil || len(result.Succeeded) != 1 {
		t.Fatalf("bulk unpublish: %+v %v", result, err)
	}
	stories, err = svc.AdminMedia(ctx, coursedomain.KindStory)
	if err != nil {
		t.Fatal(err)
	}
	for _, story := range stories {
		if story.ID != second.ID {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(story.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["published"] != false {
			t.Fatalf("story was not unpublished: %+v", payload)
		}
	}
	result, err = svc.BulkStories(ctx, StoryBulkInput{IDs: []string{first.ID, second.ID}, Action: "delete"})
	if err != nil || len(result.Succeeded) != 2 {
		t.Fatalf("bulk delete: %+v %v", result, err)
	}
	stories, err = svc.AdminMedia(ctx, coursedomain.KindStory)
	if err != nil || len(stories) != 0 {
		t.Fatalf("deleted stories remain: %v, %d", err, len(stories))
	}
	items, err = svc.AdminMedia(ctx, "")
	if err != nil || len(items) != 1 || items[0].ID != film.ID {
		t.Fatalf("film was changed: %v, %+v", err, items)
	}
}
