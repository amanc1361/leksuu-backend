package repo

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/course/domain"
)

func TestMediaCatalogIsolatesLanguageAndPublication(t *testing.T) {
	ctx := context.Background()
	r := NewMemoryCourseRepo()
	add := func(courseID, language, itemID string, published bool) {
		t.Helper()
		course := &domain.Course{ID: courseID, Language: language, CEFR: "A1", Published: published}
		if err := r.CreateCourse(ctx, course); err != nil { t.Fatal(err) }
		lesson := &domain.Lesson{ID: "lesson-" + itemID, CourseID: courseID}
		if err := r.CreateLesson(ctx, lesson); err != nil { t.Fatal(err) }
		payload := []byte(`{"language":"` + language + `","published":true,"transcript":[{"text":"hello"}]}`)
		if err := r.CreateItem(ctx, &domain.LessonItem{ID: itemID, LessonID: lesson.ID, Kind: domain.KindStory, Payload: payload}); err != nil { t.Fatal(err) }
	}
	add("course-media-de", "de", "de-visible", true)
	add("course-media-en", "en", "en-visible", true)
	add("course-media-de-draft", "de", "de-draft", false)
	items, total, err := r.MediaItems(ctx, domain.KindStory, "de", 50, 0)
	if err != nil { t.Fatal(err) }
	if total != 1 || len(items) != 1 || items[0].ID != "de-visible" { t.Fatalf("German catalog leaked content: total=%d items=%v", total, items) }
	items, total, err = r.MediaItems(ctx, domain.KindStory, "fr", 50, 0)
	if err != nil || total != 0 || len(items) != 0 { t.Fatalf("empty French catalog leaked content: total=%d items=%v err=%v", total, items, err) }
}
