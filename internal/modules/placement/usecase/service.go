// Package usecase implements the placement test: a fixed 15-question ladder
// (3 per CEFR level, A1..C1) graded server-side. A level counts as passed with
// at least 2 of its 3 questions correct; the placement is the highest
// consecutively passed level, so one lucky guess high up can't skip rungs.
package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/placement/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Levels in ladder order.
var Levels = []string{"A1", "A2", "B1", "B2", "C1"}

// question is a bank entry; Correct never leaves the server.
type question struct {
	ID      string
	Level   string
	Prompt  string
	Options [4]string
	Correct int
}

// The bank is deliberately small and hand-curated: grammar + vocabulary items
// with one defensibly correct answer each, phrased so distractors are plausible
// at that level. English-only by design (it is an English placement).
var bank = []question{
	// A1
	{"a1-1", "A1", "She ___ a teacher.", [4]string{"are", "is", "am", "be"}, 1},
	{"a1-2", "A1", "I have two ___.", [4]string{"brother", "brotheres", "brothers", "brother's"}, 2},
	{"a1-3", "A1", "___ you like coffee?", [4]string{"Do", "Does", "Are", "Is"}, 0},
	// A2
	{"a2-1", "A2", "I ___ to the cinema yesterday.", [4]string{"go", "gone", "going", "went"}, 3},
	{"a2-2", "A2", "She is ___ than her sister.", [4]string{"more tall", "taller", "tallest", "most tall"}, 1},
	{"a2-3", "A2", "We ___ dinner when the phone rang.", [4]string{"had", "have", "were having", "are having"}, 2},
	// B1
	{"b1-1", "B1", "I have lived here ___ 2019.", [4]string{"for", "since", "from", "during"}, 1},
	{"b1-2", "B1", "If it rains, we ___ at home.", [4]string{"will stay", "stayed", "would stay", "stay will"}, 0},
	{"b1-3", "B1", "The report must ___ by Friday.", [4]string{"finish", "be finish", "finished", "be finished"}, 3},
	// B2
	{"b2-1", "B2", "I wish I ___ more time to travel.", [4]string{"have", "had", "would have", "having"}, 1},
	{"b2-2", "B2", "Hardly ___ the door when the phone rang.", [4]string{"I had closed", "I closed", "had I closed", "did I closed"}, 2},
	{"b2-3", "B2", "The meeting was called off — 'called off' means ___.", [4]string{"delayed", "cancelled", "shortened", "recorded"}, 1},
	// C1
	{"c1-1", "C1", "___ the weather, the event went ahead.", [4]string{"Despite of", "Even", "Whereas", "Notwithstanding"}, 3},
	{"c1-2", "C1", "Her argument was so ___ that nobody objected.", [4]string{"compulsive", "compelling", "compelled", "compulsory"}, 1},
	{"c1-3", "C1", "No sooner ___ than the audience began to applaud.", [4]string{"had she finished", "she had finished", "she finished", "did she finished"}, 0},
}

// QuestionView is the client-safe question shape (no Correct).
type QuestionView struct {
	ID      string    `json:"id"`
	Level   string    `json:"level"`
	Prompt  string    `json:"prompt"`
	Options [4]string `json:"options"`
}

// LevelSink pushes the tested CEFR into the user's profile so the whole app
// shows one consistent level (late-bound to the auth module; best-effort).
type LevelSink func(ctx context.Context, userID, cefr string) error

type Service struct {
	repo domain.Repository
	now  func() time.Time
	sink LevelSink
}

// SetLevelSink wires the profile updater after construction (optional dep).
func (s *Service) SetLevelSink(f LevelSink) { s.sink = f }

func NewService(repo domain.Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

// Questions returns the bank without answers, in ladder order.
func (s *Service) Questions() []QuestionView {
	out := make([]QuestionView, len(bank))
	for i, q := range bank {
		out[i] = QuestionView{ID: q.ID, Level: q.Level, Prompt: q.Prompt, Options: q.Options}
	}
	return out
}

// My returns the stored result (nil when never taken).
func (s *Service) My(ctx context.Context, userID string) (*domain.Result, error) {
	return s.repo.Get(ctx, userID)
}

// Grade computes the CEFR level for a set of answers (question id -> chosen
// option index). Exported for tests; Submit persists on top of it.
func Grade(answers map[string]int) string {
	correctByLevel := map[string]int{}
	for _, q := range bank {
		if idx, ok := answers[q.ID]; ok && idx == q.Correct {
			correctByLevel[q.Level]++
		}
	}
	placed := ""
	for _, lvl := range Levels {
		if correctByLevel[lvl] >= 2 {
			placed = lvl
			continue
		}
		break // first failed rung ends the ladder
	}
	if placed == "" {
		placed = "A1" // floor: everyone starts somewhere
	}
	return placed
}

// Submit grades the answers, stores the result, and returns it.
func (s *Service) Submit(ctx context.Context, userID string, answers map[string]int) (*domain.Result, error) {
	if strings.TrimSpace(userID) == "" || len(answers) == 0 {
		return nil, apperr.ErrBadRequest
	}
	res := &domain.Result{UserID: userID, CEFR: Grade(answers), TakenAt: s.now().UTC()}
	if err := s.repo.Upsert(ctx, res); err != nil {
		return nil, err
	}
	if s.sink != nil {
		_ = s.sink(ctx, userID, res.CEFR) // profile sync is convenience, not truth
	}
	return res, nil
}
