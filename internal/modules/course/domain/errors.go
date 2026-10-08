package domain

import "github.com/leksuu/platform/internal/shared/apperr"

var (
	ErrCourseNotFound   = apperr.New("course.not_found", apperr.KindNotFound)
	ErrUnitNotFound     = apperr.New("course.unit_not_found", apperr.KindNotFound)
	ErrLessonNotFound   = apperr.New("lesson.not_found", apperr.KindNotFound)
	ErrExerciseNotFound = apperr.New("exercise.not_found", apperr.KindNotFound)
	ErrItemNotFound     = apperr.New("course.item_not_found", apperr.KindNotFound)
	// ErrLessonSourceConflict: the target unit already holds a lesson imported
	// from the same source lesson (one copy per unit).
	ErrLessonSourceConflict = apperr.New("lesson.source_conflict", apperr.KindConflict)
	// ErrLessonLocked: the learner has no access to this lesson (subscription,
	// class plan, or an unlock is required).
	ErrLessonLocked = apperr.New("lesson.locked", apperr.KindForbidden)
	// ErrLessonEmpty: the lesson has no content yet, so it cannot be completed.
	ErrLessonEmpty = apperr.New("lesson.empty", apperr.KindConflict)
	// ErrLessonIncomplete: some exercises of the lesson were never answered.
	ErrLessonIncomplete = apperr.New("lesson.incomplete", apperr.KindConflict)
	// ErrNoAssessment: a speak exercise was submitted without a pronunciation
	// assessment recorded by the server for it.
	ErrNoAssessment = apperr.New("exercise.no_assessment", apperr.KindConflict)
)
