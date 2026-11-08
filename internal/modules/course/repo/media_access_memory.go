package repo

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/leksuu/platform/internal/modules/course/domain"
)

func (r *MemoryCourseRepo) PlaybackItem(_ context.Context, id string) (*domain.LessonItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, items := range r.items {
		for _, it := range items {
			if it.ID == id && (it.Kind == domain.KindVideo || it.Kind == domain.KindMusic) {
				copy := it
				return &copy, nil
			}
		}
	}
	return nil, domain.ErrItemNotFound
}

func (r *MemoryCourseRepo) MediaLibraryBucket(_ context.Context, id string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for lessonID, items := range r.items {
		for _, it := range items {
			if it.ID == id {
				courseID := r.lessonCourse[lessonID]
				for _, course := range r.courses {
					if course.ID == courseID {
						return course.Title == "__media_library__", nil
					}
				}
			}
		}
	}
	return false, domain.ErrItemNotFound
}

func (r *MemoryCourseRepo) ProtectedMediaKey(_ context.Context, key string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, items := range r.items {
		for _, it := range items {
			if it.Kind != domain.KindVideo && it.Kind != domain.KindMusic {
				continue
			}
			var payload struct {
				URL            string `json:"url"`
				HLSManifestKey string `json:"hls_manifest_key"`
			}
			_ = json.Unmarshal(it.Payload, &payload)
			if (it.MediaKey != nil && *it.MediaKey == key) || payload.HLSManifestKey == key || strings.HasSuffix(strings.Split(payload.URL, "?")[0], "/media/"+key) {
				return true, nil
			}
		}
	}
	return false, nil
}
