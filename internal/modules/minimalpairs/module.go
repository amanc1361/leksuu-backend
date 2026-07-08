// Package minimalpairs serves a curated catalog of pronunciation minimal-pair
// drills targeting the English sounds Persian speakers most often confuse
// (w/v, th/s, th/t, æ/e, short/long i). Content is read-only; production
// scoring reuses the pronunciation module, and listening discrimination is
// graded client-side.
package minimalpairs

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/minimalpairs/domain"
	"github.com/leksuu/platform/internal/modules/minimalpairs/rest"
	"github.com/leksuu/platform/internal/modules/minimalpairs/usecase"
)

type Deps struct{ Repo domain.Repository }

type Module struct{ handler *rest.Handler }

func New(d Deps) *Module {
	return &Module{handler: rest.NewHandler(usecase.NewService(d.Repo))}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix) }
