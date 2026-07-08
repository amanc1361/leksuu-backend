package domain

import "github.com/leksuu/platform/internal/shared/apperr"

var (
	ErrCourseNotFound   = apperr.New("course.not_found", apperr.KindNotFound)
	ErrUnitNotFound     = apperr.New("course.unit_not_found", apperr.KindNotFound)
	ErrLessonNotFound   = apperr.New("lesson.not_found", apperr.KindNotFound)
	ErrExerciseNotFound = apperr.New("exercise.not_found", apperr.KindNotFound)
	ErrItemNotFound     = apperr.New("course.item_not_found", apperr.KindNotFound)
)
