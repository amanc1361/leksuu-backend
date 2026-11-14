package domain

import (
	"context"
	"time"
)

// OnboardingAnswers are the learner's one-time setup selections. These fields
// belong to the account, never the browser. Empty answers are valid when the
// learner deliberately skips the questionnaire.
type OnboardingAnswers struct {
	Reason   string `json:"reason,omitempty"`
	Level    string `json:"level,omitempty"`
	Goal     int    `json:"goal,omitempty"`
	Target   string `json:"target,omitempty"`
	UILang   string `json:"uiLang,omitempty"`
	WantTest bool   `json:"wantTest,omitempty"`
}

type OnboardingState struct {
	Completed   bool              `json:"completed"`
	Skipped     bool              `json:"skipped"`
	Answers     OnboardingAnswers `json:"answers"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
}

type OnboardingRepository interface {
	GetOnboarding(ctx context.Context, userID string) (OnboardingState, error)
	CompleteOnboarding(ctx context.Context, userID string, answers OnboardingAnswers, skipped bool) (OnboardingState, error)
}
