package usecase

import (
	"context"
	"encoding/json"
	"strings"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// StoryBulkInput applies one operation to story items in the hidden media library.
// Changes are optional; an omitted field is left as it was.
type StoryBulkInput struct {
	IDs        []string `json:"ids"`
	Action     string   `json:"action"`
	Level      *string  `json:"level,omitempty"`
	Collection *string  `json:"collection,omitempty"`
	Audience   *string  `json:"audience,omitempty"`
}

type StoryBulkFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type StoryBulkResult struct {
	Succeeded []string           `json:"succeeded"`
	Failed    []StoryBulkFailure `json:"failed"`
}

func (s *Service) BulkStories(ctx context.Context, in StoryBulkInput) (StoryBulkResult, error) {
	result := StoryBulkResult{Succeeded: []string{}, Failed: []StoryBulkFailure{}}
	if len(in.IDs) == 0 || len(in.IDs) > 100 {
		return result, apperr.ErrBadRequest
	}
	switch in.Action {
	case "publish", "unpublish", "delete":
		if in.Level != nil || in.Collection != nil || in.Audience != nil {
			return result, apperr.ErrBadRequest
		}
	case "edit":
		if in.Level == nil && in.Collection == nil && in.Audience == nil {
			return result, apperr.ErrBadRequest
		}
		if in.Level != nil {
			switch strings.TrimSpace(*in.Level) {
			case "", "A1", "A2", "B1", "B2", "C1", "C2":
			default:
				return result, apperr.ErrBadRequest
			}
		}
	default:
		return result, apperr.ErrBadRequest
	}

	// Validate every ID before changing any row. Restrict this endpoint to story
	// items in the media bucket, so course content cannot be changed by mistake.
	available, err := s.AdminMedia(ctx, coursedomain.KindStory)
	if err != nil {
		return result, err
	}
	byID := make(map[string]coursedomain.LessonItem, len(available))
	for _, item := range available {
		byID[item.ID] = item
	}
	seen := make(map[string]bool, len(in.IDs))
	items := make([]coursedomain.LessonItem, 0, len(in.IDs))
	for _, id := range in.IDs {
		item, ok := byID[id]
		if !ok || strings.TrimSpace(id) == "" || seen[id] {
			return result, apperr.ErrBadRequest
		}
		seen[id] = true
		items = append(items, item)
	}

	for _, item := range items {
		var err error
		if in.Action == "delete" {
			err = s.repo.DeleteItem(ctx, item.ID)
		} else {
			payload := map[string]any{}
			err = json.Unmarshal(item.Payload, &payload)
			if err == nil {
				switch in.Action {
				case "publish":
					payload["published"] = true
				case "unpublish":
					payload["published"] = false
				case "edit":
					if in.Level != nil {
						payload["level"] = strings.TrimSpace(*in.Level)
					}
					if in.Collection != nil {
						payload["collection"] = strings.TrimSpace(*in.Collection)
					}
					if in.Audience != nil {
						payload["audience"] = strings.TrimSpace(*in.Audience)
					}
				}
				var body []byte
				body, err = json.Marshal(payload)
				if err == nil {
					err = guardPublication(item.Payload, body)
				}
				if err == nil {
					var updated bool
					updated, err = s.repo.UpdateItemPayloadIfUnchanged(ctx, item.ID, item.Payload, body)
					if err == nil && !updated {
						err = ErrMediaJobConflict
					}
				}
			}
		}
		if err != nil {
			result.Failed = append(result.Failed, StoryBulkFailure{ID: item.ID, Error: "Could not apply story action"})
		} else {
			result.Succeeded = append(result.Succeeded, item.ID)
		}
	}
	return result, nil
}
