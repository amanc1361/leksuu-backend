package domain

import "time"

// Progress entity types and statuses (mirror user_progress CHECK semantics).
const (
	EntityLesson   = "lesson"
	EntityExercise = "exercise"

	StatusStarted   = "started"
	StatusCompleted = "completed"
)

// ProgressEntry is a single learning event recorded for a user.
type ProgressEntry struct {
	UserID     string
	EntityType string
	EntityID   string
	Status     string
	Score      *float64
	OccurredAt time.Time
}
