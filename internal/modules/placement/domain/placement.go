// Package domain holds the placement bounded context: a short laddered CEFR
// test whose result personalises course and library recommendations.
package domain

import (
	"context"
	"time"
)

// Result is a user's stored placement outcome.
type Result struct {
	UserID  string    `json:"-"`
	CEFR    string    `json:"cefr"`
	TakenAt time.Time `json:"taken_at"`
}

// Repository persists placement results (one per user, retakes overwrite).
type Repository interface {
	Get(ctx context.Context, userID string) (*Result, error) // nil when never taken
	Upsert(ctx context.Context, r *Result) error
}
