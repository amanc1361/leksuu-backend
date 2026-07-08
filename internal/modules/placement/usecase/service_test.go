package usecase

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/placement/repo"
)

// answersUpTo answers every question correctly through the given level and
// wrongly above it (choosing an index != Correct).
func answersUpTo(pass map[string]bool) map[string]int {
	out := map[string]int{}
	for _, q := range bank {
		if pass[q.Level] {
			out[q.ID] = q.Correct
		} else {
			out[q.ID] = (q.Correct + 1) % 4
		}
	}
	return out
}

func TestGradeLadder(t *testing.T) {
	cases := []struct {
		pass map[string]bool
		want string
	}{
		{map[string]bool{}, "A1"}, // all wrong → floor
		{map[string]bool{"A1": true}, "A1"},
		{map[string]bool{"A1": true, "A2": true, "B1": true}, "B1"},
		{map[string]bool{"A1": true, "A2": true, "B1": true, "B2": true, "C1": true}, "C1"},
		// a lucky high rung without the rungs below must NOT count
		{map[string]bool{"A1": true, "C1": true}, "A1"},
	}
	for i, c := range cases {
		if got := Grade(answersUpTo(c.pass)); got != c.want {
			t.Fatalf("case %d: got %s want %s", i, got, c.want)
		}
	}
}

func TestGradeTwoOfThreePasses(t *testing.T) {
	// A1: two correct, one wrong → still passes A1; A2 all wrong → placed A1.
	ans := map[string]int{}
	for _, q := range bank {
		ans[q.ID] = (q.Correct + 1) % 4
	}
	ans["a1-1"] = 1 // correct
	ans["a1-3"] = 0 // correct
	if got := Grade(ans); got != "A1" {
		t.Fatalf("got %s want A1", got)
	}
}

func TestSubmitStoresAndOverwrites(t *testing.T) {
	s := NewService(repo.NewMemoryRepo(), nil)
	ctx := context.Background()
	r1, err := s.Submit(ctx, "u1", answersUpTo(map[string]bool{"A1": true, "A2": true}))
	if err != nil || r1.CEFR != "A2" {
		t.Fatalf("first: %+v err=%v", r1, err)
	}
	r2, err := s.Submit(ctx, "u1", answersUpTo(map[string]bool{"A1": true, "A2": true, "B1": true, "B2": true}))
	if err != nil || r2.CEFR != "B2" {
		t.Fatalf("retake: %+v err=%v", r2, err)
	}
	my, _ := s.My(ctx, "u1")
	if my == nil || my.CEFR != "B2" {
		t.Fatalf("stored: %+v", my)
	}
	if _, err := s.Submit(ctx, "u1", nil); err == nil {
		t.Fatal("empty answers must fail")
	}
}

func TestSubmitSyncsProfileLevel(t *testing.T) {
	s := NewService(repo.NewMemoryRepo(), nil)
	var gotUser, gotCefr string
	s.SetLevelSink(func(_ context.Context, u, c string) error { gotUser, gotCefr = u, c; return nil })
	ctx := context.Background()
	if _, err := s.Submit(ctx, "u9", answersUpTo(map[string]bool{"A1": true, "A2": true})); err != nil {
		t.Fatal(err)
	}
	if gotUser != "u9" || gotCefr != "A2" {
		t.Fatalf("sink got %q/%q", gotUser, gotCefr)
	}
}
