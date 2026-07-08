package pronunciation

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
)

func TestStubScorerDeterministicAndShaped(t *testing.T) {
	s := stubScorer{}
	in := domain.ScoreInput{Expected: "hello how are you", Lang: "en", Audio: []byte{1, 2, 3}}
	a1, _ := s.Score(context.Background(), in)
	a2, _ := s.Score(context.Background(), in)
	if a1.Overall != a2.Overall {
		t.Fatalf("not deterministic: %d vs %d", a1.Overall, a2.Overall)
	}
	if len(a1.Words) != 4 {
		t.Fatalf("want 4 word scores, got %d", len(a1.Words))
	}
	for _, w := range a1.Words {
		if w.Score < 0 || w.Score > 100 {
			t.Fatalf("score out of range: %+v", w)
		}
	}
	if a1.Overall < 0 || a1.Overall > 100 {
		t.Fatalf("overall out of range: %d", a1.Overall)
	}
}
