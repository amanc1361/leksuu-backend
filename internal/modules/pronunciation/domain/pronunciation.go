// Package domain holds pronunciation-assessment types. An Assessment scores how
// closely a learner's recording matches an expected phrase, overall and per word.
package domain

import "context"

type WordScore struct {
	Word  string `json:"word"`
	Score int    `json:"score"` // 0..100
}

type Assessment struct {
	Overall    int         `json:"overall"` // 0..100
	Transcript string      `json:"transcript"`
	Words      []WordScore `json:"words"`
}

// ScoreInput is one recording to assess against the expected phrase.
type ScoreInput struct {
	Expected    string
	Lang        string
	Audio       []byte
	ContentType string
}

// LineScore is a user's persisted pronunciation record for one transcript line.
type LineScore struct {
	Line     int `json:"line"`
	Best     int `json:"best"`
	Last     int `json:"last"`
	Attempts int `json:"attempts"`
}

// Summary aggregates a user's pronunciation across all items.
type Summary struct {
	Scored   int `json:"scored"`   // lines with at least one attempt
	Mastered int `json:"mastered"` // lines whose best >= the mastery threshold
	AvgBest  int `json:"avg_best"` // average of best scores, 0..100
}

// Repository persists pronunciation scores per (user, item, line).
type Repository interface {
	// Upsert records a new attempt and returns the updated line score plus the
	// previous best (so callers can detect a first-time threshold crossing).
	Upsert(ctx context.Context, userID, itemID string, line, score int) (LineScore, int, error)
	ByItem(ctx context.Context, userID, itemID string) ([]LineScore, error)
	// Summary aggregates all of a user's line scores. masteredThreshold is the
	// best-score cutoff that counts a line as mastered.
	Summary(ctx context.Context, userID string, masteredThreshold int) (Summary, error)
}
