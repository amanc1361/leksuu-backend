package usecase

import (
	"context"
	"strings"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

type importArchiveStore struct {
	key         string
	contentType string
	data        []byte
}

func (s *importArchiveStore) Put(_ context.Context, key, contentType string, data []byte) (string, error) {
	s.key = key
	s.contentType = contentType
	s.data = append([]byte(nil), data...)
	return "https://cdn.example/" + key, nil
}

func TestArchiveCourseImportSourceStoresJSONAndMetadata(t *testing.T) {
	repo := courserepo.NewMemoryCourseRepo()
	store := &importArchiveStore{}
	svc := NewService(repo, nil, store)

	src, err := svc.ArchiveCourseImportSource(context.Background(), ArchiveCourseImportInput{
		FileName: "../Hello Lesson.json",
		RawJSON:  []byte(`{"id":"objective_1","class":"objective"}`),
		Metadata: []byte(`{"course_id":"course-en-a1"}`),
	})
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if src.ID == "" || src.URL == "" || src.ContentBytes == 0 || src.SHA256 == "" {
		t.Fatalf("archive metadata incomplete: %+v", src)
	}
	if src.FileName != "Hello-Lesson.json" {
		t.Fatalf("file name not sanitized: %q", src.FileName)
	}
	if !strings.HasPrefix(src.StorageKey, "course-imports/") || !strings.HasSuffix(src.StorageKey, "-Hello-Lesson.json") {
		t.Fatalf("unexpected storage key: %s", src.StorageKey)
	}
	if store.contentType != "application/json" || string(store.data) != `{"id":"objective_1","class":"objective"}` {
		t.Fatalf("store received content_type=%q data=%s", store.contentType, string(store.data))
	}
	got, total, err := svc.CourseImportSources(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(got) != 1 || got[0].StorageKey != src.StorageKey {
		t.Fatalf("source was not listed: total=%d sources=%+v", total, got)
	}
}

func TestArchiveCourseImportSourceRejectsInvalidJSON(t *testing.T) {
	store := &importArchiveStore{}
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil, store)
	if _, err := svc.ArchiveCourseImportSource(context.Background(), ArchiveCourseImportInput{
		FileName: "bad.json",
		RawJSON:  []byte(`{"broken"`),
	}); err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if store.key != "" {
		t.Fatalf("store was called for invalid JSON: %s", store.key)
	}
}
