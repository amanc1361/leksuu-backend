package pronunciation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
)

func TestHTTPScorerRequiresExplicitOverall(t *testing.T) {
	for _, body := range []string{`{}`, `{"overall":null}`, `{"transcript":"hello"}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			scorer := httpScorer{url: server.URL, client: server.Client()}
			if _, err := scorer.Score(context.Background(), domain.ScoreInput{Expected: "hello", Audio: []byte{1}}); err == nil {
				t.Fatal("missing score must not become a zero-valued assessment")
			}
		})
	}
}

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
