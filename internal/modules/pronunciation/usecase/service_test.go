package usecase

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
)

type fakeScorer struct{}

func (fakeScorer) Name() string { return "fake" }
func (fakeScorer) Score(context.Context, domain.ScoreInput) (*domain.Assessment, error) {
	return &domain.Assessment{Overall: 90}, nil
}

func TestScoreRejectsMissingInput(t *testing.T) {
	svc := NewService(fakeScorer{}, nil, nil)
	if _, _, err := svc.Score(context.Background(), "u", "", 0, domain.ScoreInput{Expected: "hi"}); err == nil {
		t.Fatal("expected error when audio is empty")
	}
	if _, _, err := svc.Score(context.Background(), "u", "", 0, domain.ScoreInput{Audio: []byte{1}}); err == nil {
		t.Fatal("expected error when expected text is empty")
	}
}

type fakeRepo struct {
	best     map[int]int
	attempts map[int]int
}

func (r *fakeRepo) Upsert(_ context.Context, _, _ string, line, score int) (domain.LineScore, int, error) {
	if r.best == nil {
		r.best = map[int]int{}
		r.attempts = map[int]int{}
	}
	prev := r.best[line]
	if score > prev {
		r.best[line] = score
	}
	r.attempts[line]++
	return domain.LineScore{Line: line, Best: r.best[line], Last: score, Attempts: r.attempts[line]}, prev, nil
}
func (r *fakeRepo) Summary(context.Context, string, int) (domain.Summary, error) {
	return domain.Summary{}, nil
}

func (r *fakeRepo) ByItem(context.Context, string, string) ([]domain.LineScore, error) {
	return nil, nil
}

type countRewarder struct{ n int }

func (c *countRewarder) Award(context.Context, string, int, string) error { c.n++; return nil }

func TestScorePersistsAndRewardsOnce(t *testing.T) {
	repo := &fakeRepo{}
	rew := &countRewarder{}
	svc := NewService(fakeScorer{}, repo, rew) // fakeScorer returns Overall 90
	in := domain.ScoreInput{Expected: "hi there", Audio: []byte{1}}

	_, ls, err := svc.Score(context.Background(), "u", "item", 0, in)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if ls == nil || ls.Best != 90 || ls.Attempts != 1 {
		t.Fatalf("want best 90 attempts 1, got %+v", ls)
	}
	if rew.n != 1 {
		t.Fatalf("want 1 award on first good score, got %d", rew.n)
	}
	// Second good attempt: already above threshold -> no extra reward, best stays.
	_, ls2, _ := svc.Score(context.Background(), "u", "item", 0, in)
	if ls2.Best != 90 || ls2.Attempts != 2 {
		t.Fatalf("want best 90 attempts 2, got %+v", ls2)
	}
	if rew.n != 1 {
		t.Fatalf("want still 1 award, got %d", rew.n)
	}
}
