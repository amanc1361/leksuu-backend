package usecase

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

type MediaProgress struct {
	PositionSeconds float64  `json:"position_seconds"`
	Attempts        int      `json:"attempts"`
	CorrectAttempts int      `json:"correct_attempts"`
	CompletedCueIDs []string `json:"completed_cue_ids"`
	Completed       bool     `json:"completed"`
}

type MediaProgressRepository interface {
	LoadMediaProgress(context.Context, string, string, string) (MediaProgress, error)
	SaveMediaPosition(context.Context, string, string, string, float64, bool) error
	RecordMediaAttempt(context.Context, string, string, string, string, bool) (MediaProgress, error)
}

type mediaItemLookup interface {
	PlaybackItem(context.Context, string) (*domain.LessonItem, error)
	MediaLibraryBucket(context.Context, string) (bool, error)
}

func (s *Service) mediaProgressTarget(ctx context.Context, userID, itemID, language string) (*domain.LessonItem, error) {
	if userID == "" || language == "" {
		return nil, apperr.ErrBadRequest
	}
	lookup, ok := s.courses.(mediaItemLookup)
	if !ok {
		return nil, apperr.ErrInternal
	}
	item, err := lookup.PlaybackItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	bucket, err := lookup.MediaLibraryBucket(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	if !bucket {
		lesson, err := s.GetLesson(ctx, Viewer{UserID: userID}, item.LessonID)
		if err != nil || lesson.Locked || lesson.Language != language {
			return nil, apperr.New("media.access_denied", apperr.KindForbidden)
		}
	}
	var payload struct {
		Language  string `json:"language"`
		Published *bool  `json:"published"`
	}
	if json.Unmarshal(item.Payload, &payload) != nil || payload.Language != language || (payload.Published != nil && !*payload.Published) {
		return nil, apperr.New("media.access_denied", apperr.KindForbidden)
	}
	return item, nil
}

func (s *Service) MediaProgress(ctx context.Context, userID, itemID, language string) (MediaProgress, error) {
	var empty MediaProgress
	if _, err := s.mediaProgressTarget(ctx, userID, itemID, language); err != nil {
		return empty, err
	}
	repo, ok := s.progress.(MediaProgressRepository)
	if !ok {
		return empty, apperr.ErrInternal
	}
	return repo.LoadMediaProgress(ctx, userID, itemID, language)
}

func (s *Service) SaveMediaPosition(ctx context.Context, userID, itemID, language string, position float64, completed bool) error {
	if math.IsNaN(position) || math.IsInf(position, 0) || position < 0 || position >= 864000 {
		return apperr.ErrBadRequest
	}
	if _, err := s.mediaProgressTarget(ctx, userID, itemID, language); err != nil {
		return err
	}
	repo, ok := s.progress.(MediaProgressRepository)
	if !ok {
		return apperr.ErrInternal
	}
	return repo.SaveMediaPosition(ctx, userID, itemID, language, position, completed)
}

// The server grades against the currently published cue rather than trusting
// a client-supplied "correct" flag. Missing indices refer to whitespace words.
func (s *Service) RecordMediaAttempt(ctx context.Context, userID, itemID, language, cueID, answer string, indices []int, full bool) (MediaProgress, bool, error) {
	var empty MediaProgress
	item, err := s.mediaProgressTarget(ctx, userID, itemID, language)
	if err != nil {
		return empty, false, err
	}
	var payload struct {
		Transcript []struct {
			CueID          string `json:"cue_id"`
			Text           string `json:"text"`
			TimingStatus   string `json:"timing_status"`
			ExerciseReview string `json:"exercise_review"`
		} `json:"transcript"`
	}
	if json.Unmarshal(item.Payload, &payload) != nil {
		return empty, false, apperr.ErrBadRequest
	}
	var expected string
	found := false
	for _, cue := range payload.Transcript {
		if cue.CueID == cueID && (cue.TimingStatus == "verified" || cue.TimingStatus == "approved") && cue.ExerciseReview == "approved" {
			found = true
			if full {
				expected = cue.Text
			} else {
				words := strings.Fields(cue.Text)
				if len(indices) == 0 || len(indices) > len(words) {
					return empty, false, apperr.ErrBadRequest
				}
				parts := make([]string, len(indices))
				previous := -1
				for i, index := range indices {
					if index <= previous || index < 0 || index >= len(words) {
						return empty, false, apperr.ErrBadRequest
					}
					parts[i] = words[index]
					previous = index
				}
				expected = strings.Join(parts, " ")
			}
			break
		}
	}
	if !found || len(answer) > 4096 {
		return empty, false, apperr.ErrBadRequest
	}
	passed := normalizeMediaAnswer(answer, language) == normalizeMediaAnswer(expected, language)
	repo, ok := s.progress.(MediaProgressRepository)
	if !ok {
		return empty, false, apperr.ErrInternal
	}
	progress, err := repo.RecordMediaAttempt(ctx, userID, itemID, language, cueID, passed)
	return progress, passed, err
}

func normalizeMediaAnswer(value, language string) string {
	value = strings.ToLower(strings.TrimSpace(norm.NFKC.String(value)))
	value = strings.NewReplacer("ي", "ی", "ك", "ک", "ە", "ه", "ة", "ه").Replace(value)
	if strings.HasPrefix(language, "en") {
		value = strings.Map(func(r rune) rune {
			if unicode.Is(unicode.Mn, r) {
				return -1
			}
			return r
		}, norm.NFD.String(value))
	}
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}), " ")
}
