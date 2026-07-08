// Package ai is the module facade for AI-assisted authoring. The provider is
// chosen by config: a deterministic stub (default) or any OpenAI-compatible
// endpoint (self-hosted open model, etc.). NOTE: generated content is a *draft*
// — it is published only after an admin reviews it.
package ai

import (
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/ai/rest"
	"github.com/leksuu/platform/internal/modules/ai/usecase"
)

type Deps struct {
	ProviderKind    string // "stub" | "openai"
	BaseURL         string
	Model           string
	APIKey          string
	DisableThinking bool
	Guard           rest.Guard
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	var p usecase.Provider
	switch d.ProviderKind {
	case "openai":
		p = openAIProvider{baseURL: d.BaseURL, model: d.Model, apiKey: d.APIKey, noThink: d.DisableThinking, client: &http.Client{Timeout: 120 * time.Second}}
	default:
		p = stubProvider{}
	}
	return &Module{handler: rest.NewHandler(usecase.NewService(p)), guard: d.Guard}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
