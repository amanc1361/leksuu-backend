// Package domain models shadowing practice: how many times a learner has
// repeated each transcript line of a media item, and when a line is considered
// "activated" (moved from passive recognition to active production).
package domain

import "context"

// ActivationReps is how many repetitions activate a line (passive -> active).
const ActivationReps = 5

// Progress holds rep counts keyed by line index (as a string, for jsonb).
type Progress struct {
	ItemID string
	Reps   map[string]int
}

type Repository interface {
	Get(ctx context.Context, userID, itemID string) (*Progress, error)
	AddRep(ctx context.Context, userID, itemID, line string) (*Progress, error)
}
