// Package domain holds the group-class context: a tutor schedules a group class
// (one shared room) and multiple students enroll. The live call runs on the same
// P2P mesh as 1:1 classes (good for small groups; an SFU is the path to scale).
package domain

import (
	"context"
	"time"
)

type ClassSession struct {
	ID          string    `json:"id"`
	TutorID     string    `json:"tutor_id"` // tutor's user account id
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	StartUTC    time.Time `json:"start_utc"`
	DurationMin int       `json:"duration_min"`
	Capacity    int       `json:"capacity"`
	RoomID      string    `json:"room_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// EndUTC returns when the session is scheduled to finish.
func (c ClassSession) EndUTC() time.Time {
	return c.StartUTC.Add(time.Duration(c.DurationMin) * time.Minute)
}

type Enrollment struct {
	ClassID   string
	StudentID string
	CreatedAt time.Time
}

type Repository interface {
	CreateClass(ctx context.Context, c *ClassSession) error
	GetClass(ctx context.Context, id string) (*ClassSession, error)
	ListUpcoming(ctx context.Context, now time.Time) ([]ClassSession, error)
	ByTutor(ctx context.Context, tutorID string) ([]ClassSession, error)
	AllClasses(ctx context.Context) ([]ClassSession, error)
	ClassesForStudent(ctx context.Context, studentID string) ([]ClassSession, error)
	ClassesStarting(ctx context.Context, from, to time.Time) ([]ClassSession, error)

	Enroll(ctx context.Context, classID, studentID string) error
	Unenroll(ctx context.Context, classID, studentID string) error
	IsEnrolled(ctx context.Context, classID, studentID string) (bool, error)
	EnrollmentCount(ctx context.Context, classID string) (int, error)
	Roster(ctx context.Context, classID string) ([]string, error)
}
