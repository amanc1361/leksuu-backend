// Package admin is the module facade for content authoring. Admin-only CRUD over
// the learning tree; writes land in the same store the course module reads.
package admin

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/admin/rest"
	"github.com/leksuu/platform/internal/modules/admin/usecase"
)

type Deps struct {
	Repo  usecase.Repository
	Vocab usecase.VocabStore
	Guard rest.Guard
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	return &Module{handler: rest.NewHandler(usecase.NewService(d.Repo, d.Vocab)), guard: d.Guard}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) {
	m.handler.Mount(mux, prefix, m.guard)
}
