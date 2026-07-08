// Package domain holds the notification context. A notification stores a TYPE
// plus structured DATA (not a localized string), so the frontend can render it
// in each recipient's own language. Link is an in-app route to open on click.
package domain

import (
	"context"
	"time"
)

const (
	TypeAssignmentNew    = "assignment_new"
	TypeAssignmentGraded = "assignment_graded"
	TypeBookingNew       = "booking_new"
	TypeBookingCancelled = "booking_cancelled"
	TypeClassEnrolled    = "class_enrolled"
	TypeReviewNew        = "review_new"
	TypeTutorWelcome     = "tutor_welcome"
	TypeClassReminder    = "class_reminder"
)

type Notification struct {
	ID        string            `json:"id"`
	UserID    string            `json:"-"`
	Type      string            `json:"type"`
	Data      map[string]string `json:"data"`
	Link      string            `json:"link,omitempty"`
	Read      bool              `json:"read"`
	CreatedAt time.Time         `json:"created_at"`
}

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	List(ctx context.Context, userID string, limit, offset int) ([]Notification, error)
	UnreadCount(ctx context.Context, userID string) (int, error)
	MarkRead(ctx context.Context, userID, id string) error
	MarkAllRead(ctx context.Context, userID string) error

	// per-user, per-type delivery preferences (only explicit overrides stored)
	GetPrefs(ctx context.Context, userID string) (map[string]bool, error)
	SetPref(ctx context.Context, userID, ntype string, enabled bool) error
}
