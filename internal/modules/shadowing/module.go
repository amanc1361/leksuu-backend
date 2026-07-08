// Package shadowing is the module facade for shadowing practice progress.
package shadowing

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/shadowing/domain"
	"github.com/leksuu/platform/internal/modules/shadowing/rest"
	"github.com/leksuu/platform/internal/modules/shadowing/usecase"
)

type Deps struct {
	Repo     domain.Repository
	Rewarder usecase.Rewarder
	Guard    rest.Guard
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	return &Module{handler: rest.NewHandler(usecase.NewService(d.Repo, d.Rewarder)), guard: d.Guard}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
