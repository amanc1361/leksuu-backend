// Package srs is the module facade for the spaced-repetition bounded context.
package srs

import (
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/modules/srs/rest"
	"github.com/leksuu/platform/internal/modules/srs/usecase"
)

type Deps struct {
	Repo     domain.Repository
	Guard    rest.Guard
	Rewarder usecase.Rewarder // optional; nil skips XP rewards
	Clock    func() time.Time
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo, d.Rewarder, d.Clock)
	return &Module{
		svc:     svc,
		handler: rest.NewHandler(svc),
		guard:   d.Guard,
	}
}

func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) {
	m.handler.Mount(mux, prefix, m.guard)
}
