// Package recording wires the class-recording bounded context: host-uploaded,
// chunk-appended, stored-once webm files served like any other media.
package recording

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/recording/domain"
	"github.com/leksuu/platform/internal/modules/recording/rest"
	"github.com/leksuu/platform/internal/modules/recording/usecase"
)

type Deps struct {
	Repo   domain.Repository
	Auth   domain.ClassAuth
	Store  domain.BlobStore
	TmpDir string
	Guard  rest.Guard
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo, d.Auth, d.Store, d.TmpDir)
	return &Module{svc: svc, handler: rest.NewHandler(svc), guard: d.Guard}
}

func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
