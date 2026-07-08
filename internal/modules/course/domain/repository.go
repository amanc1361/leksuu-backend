package domain

import "context"

// CourseRepository abstracts the learning tree. List/order methods MUST return
// lessons and items ordered by Position.
type CourseRepository interface {
	ListCourses(ctx context.Context, language, cefr string) ([]Course, error)
	Course(ctx context.Context, id string) (*Course, error)
	Lessons(ctx context.Context, courseID string) ([]Lesson, error)
	Lesson(ctx context.Context, id string) (*Lesson, error)
	Items(ctx context.Context, lessonID string) ([]LessonItem, error)
	MediaItems(ctx context.Context, kind, language string, limit, offset int) ([]LessonItem, int, error)
	Exercises(ctx context.Context, itemID string) ([]Exercise, error)
	Exercise(ctx context.Context, id string) (*Exercise, error)
	LessonCount(ctx context.Context, courseID string) (int, error)
	UpdateItem(ctx context.Context, it *LessonItem) error
	DeleteItem(ctx context.Context, itemID string) error
	ReorderItems(ctx context.Context, lessonID string, orderedIDs []string) error
	// units (chapters)
	Units(ctx context.Context, courseID string) ([]Unit, error)
	Unit(ctx context.Context, id string) (*Unit, error)
	CreateUnit(ctx context.Context, u *Unit) error
	LessonsByUnit(ctx context.Context, unitID string) ([]Lesson, error)
	ReorderUnits(ctx context.Context, courseID string, orderedIDs []string) error
	ReorderLessons(ctx context.Context, unitID string, orderedIDs []string) error
	SearchCourses(ctx context.Context, q string, limit, offset int) ([]Course, int, error)
	// Access rules (granular per-entity subscription gating).
	GetAccessRule(ctx context.Context, entityType, entityID string) (*AccessRule, error)
	SetAccessRule(ctx context.Context, rule AccessRule) error
	DeleteAccessRule(ctx context.Context, entityType, entityID string) error
	ListAccessRules(ctx context.Context) ([]AccessRule, error)
}

// ProgressRepository records and summarizes user learning events.
type ProgressRepository interface {
	Record(ctx context.Context, e ProgressEntry) error
	CompletedLessons(ctx context.Context, userID, courseID string) (int, error)
	// CompletedLessonIDs returns the ids of completed lessons in a course (for
	// per-lesson progress state in the UI).
	CompletedLessonIDs(ctx context.Context, userID, courseID string) ([]string, error)
	// ExerciseAttempts counts how many times the user has already attempted this
	// exercise. Used to award XP only on a first-attempt-correct answer.
	ExerciseAttempts(ctx context.Context, userID, exerciseID string) (int, error)
}
