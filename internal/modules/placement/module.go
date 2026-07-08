// Package placement wires the CEFR placement test module.
package placement

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/placement/domain"
	"github.com/leksuu/platform/internal/modules/placement/rest"
	"github.com/leksuu/platform/internal/modules/placement/usecase"
)

type Deps struct {
	Repo  domain.Repository
	Guard rest.Guard
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo, nil)
	return &Module{svc: svc, handler: rest.NewHandler(svc), guard: d.Guard}
}

func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
