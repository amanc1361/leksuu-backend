// Package domain is the SRS (spaced repetition) bounded context. It implements a
// Leitner box scheduler over per-user cards backed by shared vocabulary items —
// the "click a word/phrase → review later" loop (Phase 1).
package domain

import (
	"context"
	"time"

	"github.com/leksuu/platform/internal/shared/apperr"
)

// Grade is the learner's self-assessment after seeing a card's answer.
type Grade string

const (
	GradeAgain Grade = "again" // forgot -> relearn now, ease down, lapse
	GradeHard  Grade = "hard"  // recalled with difficulty -> small interval growth, ease down
	GradeGood  Grade = "good"  // recalled -> interval * ease
	GradeEasy  Grade = "easy"  // trivial -> larger interval, ease up
)

func (g Grade) Valid() bool {
	return g == GradeAgain || g == GradeHard || g == GradeGood || g == GradeEasy
}

// ErrCardNotFound is returned when a card is missing or not owned by the user.
var ErrCardNotFound = apperr.New("srs.card_not_found", apperr.KindNotFound)

// ErrVocabNotFound is returned when a vocabulary item is missing.
var ErrVocabNotFound = apperr.New("srs.vocab_not_found", apperr.KindNotFound)

// MaxBox is the number of maturity buckets shown in the UI histogram. Boxes are
// derived from the current interval (a maturity proxy) rather than driving the
// schedule directly — the real scheduling is an SM-2-style ease model below.
const MaxBox = 5

// Scheduling constants (SM-2 variant operating in whole days).
const (
	EaseDefault = 2.5 // a fresh card's ease factor
	EaseMin     = 1.3 // floor: a hard card never collapses below this
	EaseMax     = 3.0 // cap: avoid runaway intervals on always-easy cards
	MaxInterval = 365 // a single review never schedules further than a year out
)

// ease adjustment per grade (added to the card's ease, then clamped).
const (
	easeDeltaAgain = -0.20
	easeDeltaHard  = -0.15
	easeDeltaGood  = 0.0
	easeDeltaEasy  = +0.15
)

// SchedInput is a card's pre-review scheduling state.
type SchedInput struct {
	IntervalDays int
	Reps         int
	Lapses       int
	Ease         float64
}

// SchedResult is the post-review scheduling state.
type SchedResult struct {
	Box          int
	IntervalDays int
	Reps         int
	Lapses       int
	Ease         float64
	Due          time.Time
	// Suspend is true when this review pushed the card to/over the leech
	// threshold via "again" — the caller should auto-suspend it from review.
	Suspend bool
}

func clampEase(e float64) float64 {
	if e < EaseMin {
		return EaseMin
	}
	if e > EaseMax {
		return EaseMax
	}
	return e
}

func roundDays(x float64) int {
	n := int(x + 0.5)
	if n < 1 {
		n = 1
	}
	if n > MaxInterval {
		n = MaxInterval
	}
	return n
}

// boxFromInterval projects an interval (days) onto a 1..MaxBox maturity bucket
// so the UI's box histogram stays meaningful under the ease model.
func boxFromInterval(iv int) int {
	switch {
	case iv <= 0:
		return 1
	case iv < 3:
		return 2
	case iv < 7:
		return 3
	case iv < 21:
		return 4
	default:
		return MaxBox
	}
}

// Reschedule applies one SM-2 step. Pure and deterministic for a given clock,
// so it is fully unit-testable. `again` lapses the card back into relearning
// (reappears in ~10 minutes); `good` grows the interval by the ease factor;
// `hard`/`easy` tighten/loosen growth and nudge the ease factor down/up.
func Reschedule(in SchedInput, g Grade, now time.Time) SchedResult {
	ease := in.Ease
	if ease <= 0 {
		ease = EaseDefault
	}
	reps, lapses := in.Reps, in.Lapses
	var iv int

	switch g {
	case GradeAgain:
		ease = clampEase(ease + easeDeltaAgain)
		lapses++
		reps = 0
		iv = 0 // relearn this session
	case GradeHard:
		ease = clampEase(ease + easeDeltaHard)
		reps++
		if in.IntervalDays < 1 {
			iv = 1
		} else {
			iv = roundDays(float64(in.IntervalDays) * 1.2)
		}
	case GradeEasy:
		ease = clampEase(ease + easeDeltaEasy)
		reps++
		if in.IntervalDays < 1 {
			iv = 4
		} else {
			iv = roundDays(float64(in.IntervalDays) * ease * 1.3)
		}
	default: // GradeGood
		ease = clampEase(ease + easeDeltaGood)
		reps++
		switch reps {
		case 1:
			iv = 1
		case 2:
			iv = 6
		default:
			iv = roundDays(float64(in.IntervalDays) * ease)
		}
	}

	var due time.Time
	if iv <= 0 {
		due = now.Add(10 * time.Minute)
	} else {
		due = now.AddDate(0, 0, iv)
	}
	return SchedResult{
		Box:          boxFromInterval(iv),
		IntervalDays: iv,
		Reps:         reps,
		Lapses:       lapses,
		Ease:         ease,
		Due:          due,
		Suspend:      g == GradeAgain && lapses >= LeechLapses,
	}
}

// PreviewIntervals returns the interval (in days) each grade would produce from
// the given state, so the client can show "good · 6d" on the buttons. `again`
// is reported as 0 (relearn this session).
func PreviewIntervals(in SchedInput, now time.Time) map[string]int {
	out := make(map[string]int, 4)
	for _, g := range []Grade{GradeAgain, GradeHard, GradeGood, GradeEasy} {
		out[string(g)] = Reschedule(in, g, now).IntervalDays
	}
	return out
}

// Vocab is a shared dictionary entry (word/phrase/chunk). Meaning is localized.
type Vocab struct {
	ID            string
	Language      string
	Kind          string
	Lemma         string
	Pronunciation string
	Meaning       map[string]string
	Example       string
	AudioKey      string
}

// Card is one user's review state for a vocab item.
// Clip references a moment in a media item so the word can be re-played in its
// original context at review time (context-anchored SRS).
type Clip struct {
	URL   string  `json:"url"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type Card struct {
	ID           string
	UserID       string
	VocabID      string
	Box          int
	IntervalDays int
	Ease         float64
	Reps         int
	Lapses       int
	Suspended    bool
	DueAt        time.Time
	Source       string
	Clip         *Clip
	// Prev holds the pre-review scheduling state for one-level undo; nil when
	// there is nothing to undo (fresh card, or undo already used).
	Prev *CardSnapshot
}

// CardSnapshot is the schedule state captured before a review, restored by undo.
type CardSnapshot struct {
	Box          int       `json:"box"`
	IntervalDays int       `json:"interval_days"`
	Ease         float64   `json:"ease"`
	Reps         int       `json:"reps"`
	Lapses       int       `json:"lapses"`
	Suspended    bool      `json:"suspended"`
	DueAt        time.Time `json:"due_at"`
}

// DueCard is a review-ready projection: card state joined with its vocab so the
// client can show the front (lemma) and reveal the back (localized meaning).
type DueCard struct {
	CardID   string            `json:"card_id"`
	Box      int               `json:"box"`
	DueAt    time.Time         `json:"due_at"`
	Language string            `json:"language"`
	Lemma    string            `json:"lemma"`
	Meaning  map[string]string `json:"meaning"`
	Example  string            `json:"example,omitempty"`
	Clip     *Clip             `json:"clip,omitempty"`
	// Scheduling state (used to compute Preview; not exposed raw).
	IntervalDays int     `json:"-"`
	Reps         int     `json:"-"`
	Lapses       int     `json:"-"`
	Ease         float64 `json:"-"`
	// Preview maps each grade to the interval (days) it would schedule next.
	Preview map[string]int `json:"preview,omitempty"`
	// Leech flags a card the learner keeps forgetting (Lapses >= LeechLapses).
	Leech bool `json:"leech,omitempty"`
}

// LeechLapses is the number of lapses at which a card is flagged a "leech" —
// a stubborn item the learner keeps forgetting, worth extra attention. Anki
// uses 8 for dense daily review; Leksuu's lighter cadence flags earlier.
const LeechLapses = 4

// LeechCard is a stubborn card surfaced on the maturity dashboard.
type LeechCard struct {
	CardID    string            `json:"card_id"`
	Lemma     string            `json:"lemma"`
	Meaning   map[string]string `json:"meaning"`
	Lapses    int               `json:"lapses"`
	Box       int               `json:"box"`
	Suspended bool              `json:"suspended"`
	Source    string            `json:"source,omitempty"`
}

// Activity drives the daily review goal + review streak on the dashboard.
type Activity struct {
	Today  int `json:"today"`  // reviews completed today (UTC)
	Goal   int `json:"goal"`   // target reviews per day
	Streak int `json:"streak"` // consecutive days (ending today/yesterday) with >=1 review
}

// DayCount is one day in the 7-day review-activity strip.
type DayCount struct {
	Offset int    `json:"offset"` // -6..0 (0 = today)
	Date   string `json:"date"`   // YYYY-MM-DD (UTC)
	Count  int    `json:"count"`  // reviews that day
	Met    bool   `json:"met"`    // count >= daily goal
}

// DailyReviewGoal is the default per-day review target (used when a user
// hasn't set their own). Users can override it within [MinDailyGoal, MaxDailyGoal].
const DailyReviewGoal = 10

const (
	MinDailyGoal = 1
	MaxDailyGoal = 100
)

// Settings holds per-user SRS preferences.
type Settings struct {
	DailyGoal int `json:"daily_goal"`
}

// ClampGoal keeps a requested daily goal within the allowed range.
func ClampGoal(n int) int {
	if n < MinDailyGoal {
		return MinDailyGoal
	}
	if n > MaxDailyGoal {
		return MaxDailyGoal
	}
	return n
}

// DayForecast is the number of cards coming due on a given day. Offset 0 is
// today (overdue cards fold into today); Date is the UTC calendar day.
type DayForecast struct {
	Offset int    `json:"offset"`
	Date   string `json:"date"`
	Due    int    `json:"due"`
}

// Repository persists vocab + cards. The Postgres impl maps onto the normalized
// vocabulary_items / srs_cards tables; the memory impl mirrors that behaviour.
type Repository interface {
	EnsureVocab(ctx context.Context, v *Vocab) error                   // upsert by (language, kind, lemma); fills v.ID
	EnsureCard(ctx context.Context, c *Card) (created bool, err error) // create if (user, vocab) absent; fills c.ID
	Card(ctx context.Context, userID, cardID string) (*Card, error)
	UpdateCard(ctx context.Context, c *Card) error
	// MyCards lists the user's cards joined with vocab, newest-due first.
	// query filters by lemma substring; status: "" (all) | "active" |
	// "suspended" | "leech".
	MyCards(ctx context.Context, userID, query, status string, limit, offset int) ([]CardRow, error)
	DeleteCard(ctx context.Context, userID, cardID string) error
	Due(ctx context.Context, userID, language string, now time.Time, limit int) ([]DueCard, error)
	BoxCounts(ctx context.Context, userID string) (map[int]int, error)
	DueCount(ctx context.Context, userID string, now time.Time) (int, error)
	Forecast(ctx context.Context, userID string, now time.Time, days int) ([]DayForecast, error)
	Leeches(ctx context.Context, userID string, minLapses, limit int) ([]LeechCard, error)
	LogReview(ctx context.Context, userID string, at time.Time) error
	ReviewActivity(ctx context.Context, userID string, now time.Time) (today, streak int, err error)
	ReviewWeek(ctx context.Context, userID string, now time.Time) (map[string]int, error)
	GetGoal(ctx context.Context, userID string) (int, error)
	SetGoal(ctx context.Context, userID string, goal int) error
	// vocabulary bank management (admin) + public lookup
	ListVocab(ctx context.Context, language, q string, limit, offset int) ([]Vocab, int, error)
	GetVocab(ctx context.Context, id string) (*Vocab, error)
	UpdateVocab(ctx context.Context, v *Vocab) error
	DeleteVocab(ctx context.Context, id string) error
	LookupVocab(ctx context.Context, language, lemma string) (*Vocab, error)
}

// CardRow is the word-manager projection: card state + its vocab front/back.
type CardRow struct {
	CardID       string            `json:"card_id"`
	Lemma        string            `json:"lemma"`
	Language     string            `json:"language"`
	Meaning      map[string]string `json:"meaning"`
	Box          int               `json:"box"`
	IntervalDays int               `json:"interval_days"`
	Lapses       int               `json:"lapses"`
	Suspended    bool              `json:"suspended"`
	Leech        bool              `json:"leech"`
	DueAt        time.Time         `json:"due_at"`
}
