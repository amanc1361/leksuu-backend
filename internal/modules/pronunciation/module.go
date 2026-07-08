// Package pronunciation scores shadowing recordings behind a swappable Scorer
// port: a deterministic stub (default) or an HTTP scoring service (Whisper +
// GOP/wav2vec2) selected by config — same API either way.
package pronunciation

import (
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
	"github.com/leksuu/platform/internal/modules/pronunciation/rest"
	"github.com/leksuu/platform/internal/modules/pronunciation/usecase"
)

type Deps struct {
	ProviderKind string // "stub" | "http"
	URL          string
	APIKey       string
	Repo         domain.Repository
	Rewarder     usecase.Rewarder
	Premium      rest.Premium
	Guard        rest.Guard
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	var s usecase.Scorer
	switch d.ProviderKind {
	case "http":
		s = httpScorer{url: d.URL, apiKey: d.APIKey, client: &http.Client{Timeout: 30 * time.Second}}
	default:
		s = stubScorer{}
	}
	svc := usecase.NewService(s, d.Repo, d.Rewarder)
	return &Module{svc: svc, handler: rest.NewHandler(svc, d.Premium), guard: d.Guard}
}

func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
