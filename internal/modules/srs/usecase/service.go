package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Rewarder lets a review grant a little XP. Optional (nil skips rewards); the
// gamification service satisfies it — same port the course module uses.
type Rewarder interface {
	Award(ctx context.Context, userID string, xp int, reason string) error
}

type Service struct {
	repo     domain.Repository
	rewarder Rewarder
	clock    func() time.Time
}

func NewService(repo domain.Repository, rewarder Rewarder, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, rewarder: rewarder, clock: clock}
}

// AddInput captures what's needed to start reviewing a word/phrase.
type AddInput struct {
	Language string
	Kind     string
	Lemma    string
	Example  string
	Source   string
	Meaning  map[string]string
	Clip     *domain.Clip
}

// AddCard ensures the vocab exists and the user has a (box 1, due now) card for
// it. Idempotent: adding the same phrase twice won't duplicate it.
func (s *Service) AddCard(ctx context.Context, userID string, in AddInput) (*domain.Card, error) {
	if userID == "" || in.Lemma == "" || in.Language == "" {
		return nil, apperr.ErrBadRequest
	}
	kind := in.Kind
	if kind == "" {
		kind = "phrase"
	}
	meaning := map[string]string{}
	for k, m := range in.Meaning {
		if strings.TrimSpace(m) != "" {
			meaning[k] = m
		}
	}
	// A card saved without a native meaning (e.g. an imported phrase card)
	// borrows the dictionary entry's meaning, so review has an answer to show
	// (QA N03). Best-effort: no entry simply leaves it empty.
	if len(meaning) == 0 {
		if d, err := s.repo.LookupVocab(ctx, in.Language, domain.NormalizeLemma(in.Lemma)); err == nil && len(d.Meaning) > 0 {
			meaning = d.Meaning
		}
	}
	v := &domain.Vocab{Language: in.Language, Kind: kind, Lemma: in.Lemma, Meaning: meaning, Example: in.Example}
	if err := s.repo.EnsureVocab(ctx, v); err != nil {
		return nil, err
	}
	now := s.clock()
	c := &domain.Card{UserID: userID, VocabID: v.ID, Box: 1, IntervalDays: 0, Ease: domain.EaseDefault, DueAt: now, Source: in.Source, Clip: in.Clip}
	if _, err := s.repo.EnsureCard(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Due returns cards ready to review now, weakest-and-oldest first, each with a
// per-grade interval preview for the review buttons.
func (s *Service) Due(ctx context.Context, userID, language string, limit int) ([]domain.DueCard, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	now := s.clock()
	cards, err := s.repo.Due(ctx, userID, language, now, limit)
	if err != nil {
		return nil, err
	}
	for i := range cards {
		cards[i].Preview = domain.PreviewIntervals(domain.SchedInput{
			IntervalDays: cards[i].IntervalDays,
			Reps:         cards[i].Reps,
			Lapses:       cards[i].Lapses,
			Ease:         cards[i].Ease,
		}, now)
		cards[i].Leech = cards[i].Lapses >= domain.LeechLapses
	}
	return cards, nil
}

// Review records a grade, reschedules the card with the SM-2 ease model, and
// grants a small XP reward.
func (s *Service) Review(ctx context.Context, userID, cardID string, g domain.Grade) (*domain.Card, error) {
	if !g.Valid() {
		return nil, apperr.ErrBadRequest
	}
	c, err := s.repo.Card(ctx, userID, cardID)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	due := !c.DueAt.After(now)
	// Review policy: a graduated card reviewed before it is due (a double
	// submit, a stale session, a direct call) changes nothing — its schedule
	// only advances on a scheduled review. A card in relearning (interval 0,
	// shown again in the same session after "again") may take its learning step
	// early, but only a scheduled (due) review earns XP, once: re-reviewing a
	// state restored by Undo is not rewarded again (Undo itself keeps the XP).
	if !due && c.IntervalDays > 0 {
		return c, nil
	}
	rewarded := due && !(c.Prev != nil && c.Prev.Undone && c.Prev.Rewarded && c.Prev.DueAt.Equal(c.DueAt))
	res := domain.Reschedule(domain.SchedInput{
		IntervalDays: c.IntervalDays,
		Reps:         c.Reps,
		Lapses:       c.Lapses,
		Ease:         c.Ease,
	}, g, now)
	// Capture the pre-review state so the learner can undo a misgrade.
	c.Prev = &domain.CardSnapshot{
		Box: c.Box, IntervalDays: c.IntervalDays, Ease: c.Ease,
		Reps: c.Reps, Lapses: c.Lapses, Suspended: c.Suspended, DueAt: c.DueAt,
		Rewarded: rewarded,
	}
	c.Box, c.IntervalDays, c.Reps, c.Lapses, c.Ease, c.DueAt = res.Box, res.IntervalDays, res.Reps, res.Lapses, res.Ease, res.Due
	if res.Suspend {
		c.Suspended = true // stubborn card: stop drilling until the learner relearns it
	}
	if err := s.repo.UpdateCard(ctx, c); err != nil {
		return nil, err
	}
	_ = s.repo.LogReview(ctx, userID, now) // best-effort; powers the daily goal + streak
	if rewarded && s.rewarder != nil {
		_ = s.rewarder.Award(ctx, userID, 2, "srs_review")
	}
	return c, nil
}

// Relearn resumes a suspended (or any) card and resets it to a fresh learning
// state — a clean restart for a stubborn card, instead of endless drilling.
func (s *Service) Relearn(ctx context.Context, userID, cardID string) (*domain.Card, error) {
	c, err := s.repo.Card(ctx, userID, cardID)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	c.Box, c.IntervalDays, c.Reps, c.Lapses, c.Ease = 1, 0, 0, 0, domain.EaseDefault
	c.Suspended = false
	c.DueAt = now
	if err := s.repo.UpdateCard(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Activity returns today's review count, the user's daily goal, and the streak.
func (s *Service) Activity(ctx context.Context, userID string) (*domain.Activity, error) {
	today, streak, err := s.repo.ReviewActivity(ctx, userID, s.clock())
	if err != nil {
		return nil, err
	}
	goal, err := s.repo.GetGoal(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &domain.Activity{Today: today, Goal: goal, Streak: streak}, nil
}

// WeekActivity returns the last 7 days of review counts (oldest first), each
// flagged as goal-met, for the dashboard activity strip.
func (s *Service) WeekActivity(ctx context.Context, userID string) ([]domain.DayCount, error) {
	now := s.clock()
	counts, err := s.repo.ReviewWeek(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	goal, err := s.repo.GetGoal(ctx, userID)
	if err != nil {
		return nil, err
	}
	nowU := now.UTC()
	today := time.Date(nowU.Year(), nowU.Month(), nowU.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]domain.DayCount, 0, 7)
	for off := -6; off <= 0; off++ {
		d := today.AddDate(0, 0, off).Format("2006-01-02")
		c := counts[d]
		out = append(out, domain.DayCount{Offset: off, Date: d, Count: c, Met: goal > 0 && c >= goal})
	}
	return out, nil
}

// Settings returns the user's SRS preferences (currently the daily goal).
func (s *Service) Settings(ctx context.Context, userID string) (*domain.Settings, error) {
	goal, err := s.repo.GetGoal(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &domain.Settings{DailyGoal: goal}, nil
}

// SetGoal updates the user's daily review goal (clamped to the allowed range).
func (s *Service) SetGoal(ctx context.Context, userID string, goal int) (*domain.Settings, error) {
	g := domain.ClampGoal(goal)
	if err := s.repo.SetGoal(ctx, userID, g); err != nil {
		return nil, err
	}
	return &domain.Settings{DailyGoal: g}, nil
}

// Stats is the at-a-glance review state for dashboards.
type Stats struct {
	Due   int         `json:"due"`
	Total int         `json:"total"`
	Boxes map[int]int `json:"boxes"`
}

func (s *Service) Stats(ctx context.Context, userID string) (*Stats, error) {
	boxes, err := s.repo.BoxCounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	due, err := s.repo.DueCount(ctx, userID, s.clock())
	if err != nil {
		return nil, err
	}
	total := 0
	for _, n := range boxes {
		total += n
	}
	return &Stats{Due: due, Total: total, Boxes: boxes}, nil
}

// Forecast returns the per-day review load for the next `days` days (today
// first; overdue cards fold into today). Used by the maturity dashboard.
func (s *Service) Forecast(ctx context.Context, userID string, days int) ([]domain.DayForecast, error) {
	if days <= 0 || days > 30 {
		days = 14
	}
	return s.repo.Forecast(ctx, userID, s.clock(), days)
}

// Leeches lists the learner's most stubborn cards (Lapses >= LeechLapses),
// hardest first, for the maturity dashboard's "needs attention" section.
func (s *Service) Leeches(ctx context.Context, userID string, limit int) ([]domain.LeechCard, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.Leeches(ctx, userID, domain.LeechLapses, limit)
}

// Lookup returns a vocabulary bank entry for click-word fallback (public).
// ListVocab returns vocabulary from the bank (for browsing and in-class push).
func (s *Service) ListVocab(ctx context.Context, language, q string, limit, offset int) ([]domain.Vocab, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListVocab(ctx, language, q, limit, offset)
}

func (s *Service) Lookup(ctx context.Context, language, lemma string) (*domain.Vocab, error) {
	key := domain.NormalizeLemma(lemma)
	if language == "" || key == "" {
		return nil, domain.ErrVocabNotFound
	}
	return s.repo.LookupVocab(ctx, language, key)
}

// Undo restores the card's schedule to what it was before the last review
// (one level, Anki-style). XP from the reviewed attempt is intentionally kept:
// the recall attempt genuinely happened — undo only fixes the schedule. The
// restored state is marked so reviewing it again does not earn XP twice.
func (s *Service) Undo(ctx context.Context, userID, cardID string) (*domain.Card, error) {
	c, err := s.repo.Card(ctx, userID, cardID)
	if err != nil {
		return nil, err
	}
	if c.Prev == nil || c.Prev.Undone {
		return nil, apperr.New("srs.nothing_to_undo", apperr.KindConflict)
	}
	p := c.Prev
	c.Box, c.IntervalDays, c.Ease, c.Reps, c.Lapses, c.Suspended, c.DueAt =
		p.Box, p.IntervalDays, p.Ease, p.Reps, p.Lapses, p.Suspended, p.DueAt
	c.Prev = &domain.CardSnapshot{Undone: true, Rewarded: p.Rewarded, DueAt: p.DueAt}
	if err := s.repo.UpdateCard(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// MyCards is the learner's word-manager listing.
func (s *Service) MyCards(ctx context.Context, userID, query, status string, limit, offset int) ([]domain.CardRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.MyCards(ctx, userID, query, status, limit, offset)
}

// Suspend pauses/resumes a card manually. Unlike Relearn it preserves the
// schedule, so resuming picks up where the learner left off.
func (s *Service) Suspend(ctx context.Context, userID, cardID string, suspended bool) (*domain.Card, error) {
	c, err := s.repo.Card(ctx, userID, cardID)
	if err != nil {
		return nil, err
	}
	c.Suspended = suspended
	if err := s.repo.UpdateCard(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// DeleteCard removes a card (its shared vocabulary entry stays).
func (s *Service) DeleteCard(ctx context.Context, userID, cardID string) error {
	return s.repo.DeleteCard(ctx, userID, cardID)
}
