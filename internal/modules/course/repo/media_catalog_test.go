package repo

import (
	"context"
	"fmt"
	"testing"

	"github.com/leksuu/platform/internal/modules/course/domain"
)

func TestMediaCatalogIsolatesLanguageAndPublication(t *testing.T) {
	ctx := context.Background()
	r := NewMemoryCourseRepo()
	add := func(courseID, language, itemID string, published bool) {
		t.Helper()
		course := &domain.Course{ID: courseID, Language: language, CEFR: "A1", Published: published}
		if err := r.CreateCourse(ctx, course); err != nil {
			t.Fatal(err)
		}
		lesson := &domain.Lesson{ID: "lesson-" + itemID, CourseID: courseID}
		if err := r.CreateLesson(ctx, lesson); err != nil {
			t.Fatal(err)
		}
		payload := []byte(`{"language":"` + language + `","published":true,"transcript":[{"text":"hello"}]}`)
		if err := r.CreateItem(ctx, &domain.LessonItem{ID: itemID, LessonID: lesson.ID, Kind: domain.KindStory, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	add("course-media-de", "de", "de-visible", true)
	add("course-media-en", "en", "en-visible", true)
	add("course-media-de-draft", "de", "de-draft", false)
	items, total, err := r.MediaItems(ctx, domain.KindStory, "de", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != "de-visible" {
		t.Fatalf("German catalog leaked content: total=%d items=%v", total, items)
	}
	items, total, err = r.MediaItems(ctx, domain.KindStory, "fr", 50, 0)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("empty French catalog leaked content: total=%d items=%v err=%v", total, items, err)
	}
}

// The hidden multi-language storage bucket is not a learner course. Its items
// are published separately and must still be filtered by their own language.
func TestMediaCatalogIncludesPublishedHiddenBucketItems(t *testing.T) {
	ctx := context.Background()
	r := NewMemoryCourseRepo()
	if err := r.CreateCourse(ctx, &domain.Course{ID: "bucket", Title: "__media_library__", Language: "en", Published: false}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateLesson(ctx, &domain.Lesson{ID: "bucket-lesson", CourseID: "bucket"}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		id, language string
		published    bool
	}{{"de-live", "de", true}, {"en-live", "en", true}, {"de-draft", "de", false}} {
		payload := []byte(fmt.Sprintf(`{"language":%q,"published":%t,"transcript":[{"text":"hello"}]}`, item.language, item.published))
		if err := r.CreateItem(ctx, &domain.LessonItem{ID: item.id, LessonID: "bucket-lesson", Kind: domain.KindStory, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := r.MediaItems(ctx, domain.KindStory, "de", 50, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].ID != "de-live" {
		t.Fatalf("hidden bucket visibility/language isolation failed: total=%d items=%v err=%v", total, items, err)
	}
}
