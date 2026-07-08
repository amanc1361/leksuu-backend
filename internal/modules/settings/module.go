// Package settings is the admin-editable platform settings module (prices +
// access policies), persisted in one record and read live by billing.
package settings

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/settings/domain"
	"github.com/leksuu/platform/internal/modules/settings/rest"
	"github.com/leksuu/platform/internal/modules/settings/usecase"
)

type Deps struct {
	Repo     domain.Repository
	PlanRepo domain.PlanRepository
	Guard    rest.Guard
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo)
	if d.PlanRepo != nil {
		svc.SetPlans(d.PlanRepo)
	}
	return &Module{svc: svc, handler: rest.NewHandler(svc), guard: d.Guard}
}

func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
