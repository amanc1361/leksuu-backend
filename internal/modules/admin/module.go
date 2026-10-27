// Package admin is the module facade for content authoring. Admin-only CRUD over
// the learning tree; writes land in the same store the course module reads.
package admin

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/admin/rest"
	"github.com/leksuu/platform/internal/modules/admin/usecase"
	mediadomain "github.com/leksuu/platform/internal/modules/media/domain"
)

type Deps struct {
	Repo              usecase.Repository
	Vocab             usecase.VocabStore
	Guard             rest.Guard
	LocalizationURL   string
	LocalizationToken string
	Store             mediadomain.Store
	BulkMaxBytes      int64
}

type Module struct {
	handler           *rest.Handler
	guard             rest.Guard
	localizationURL   string
	localizationToken string
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo, d.Vocab, d.Store)
	svc.SetBulkMaxBytes(d.BulkMaxBytes)
	return &Module{handler: rest.NewHandler(svc), guard: d.Guard, localizationURL: d.LocalizationURL, localizationToken: d.LocalizationToken}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) {
	m.handler.Mount(mux, prefix, m.guard)
	path := prefix + "/admin/localization/"
	mux.Handle(path, m.guard.RequireRole("admin", rest.LocalizationProxy(m.localizationURL, m.localizationToken, path)))
}
