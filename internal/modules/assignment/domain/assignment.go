// Package domain holds the homework/assignment context: a tutor assigns a piece
// of work to a student, the student submits a response, and the tutor grades it.
// One assignment is one task for one student (1:1 with its submission + grade).
package domain

import (
	"context"
	"time"
)

// Notification type strings emitted by this module (mirror notification.domain).
const (
	TypeNew    = "assignment_new"
	TypeGraded = "assignment_graded"
)

const (
	TypeTask   = "task"   // free-text instruction; student responds with text
	TypeLesson = "lesson" // points the student at an existing lesson to complete
)

const (
	StatusAssigned  = "assigned"
	StatusSubmitted = "submitted"
	StatusGraded    = "graded"
)

type Assignment struct {
	ID        string     `json:"id"`
	TutorID   string     `json:"tutor_id"` // tutor's user account id
	StudentID string     `json:"student_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Prompt    string     `json:"prompt"`
	LessonID  string     `json:"lesson_id,omitempty"`
	DueUTC    *time.Time `json:"due_utc,omitempty"`
	CreatedAt time.Time  `json:"created_at"`

	SubmissionText string     `json:"submission_text,omitempty"`
	SubmittedAt    *time.Time `json:"submitted_at,omitempty"`

	Score    int        `json:"score"` // -1 = ungraded
	Feedback string     `json:"feedback,omitempty"`
	GradedAt *time.Time `json:"graded_at,omitempty"`
}

// Status is derived from the submission/grade timestamps.
func (a Assignment) Status() string {
	switch {
	case a.GradedAt != nil:
		return StatusGraded
	case a.SubmittedAt != nil:
		return StatusSubmitted
	default:
		return StatusAssigned
	}
}

type Repository interface {
	Create(ctx context.Context, a *Assignment) error
	Get(ctx context.Context, id string) (*Assignment, error)
	ByTutor(ctx context.Context, tutorID string) ([]Assignment, error)
	ByStudent(ctx context.Context, studentID string) ([]Assignment, error)
	Update(ctx context.Context, a *Assignment) error
}
