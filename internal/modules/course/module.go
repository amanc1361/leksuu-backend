// Package course is the module facade for the learning bounded context
// (courses -> lessons -> items -> exercises + progress). It wires its own layers
// and exposes Mount. Browse endpoints are public; progress endpoints are guarded
// by an injected authenticator from the auth module.
package course

import (
	"context"
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/modules/course/rest"
	"github.com/leksuu/platform/internal/modules/course/usecase"
)

type Deps struct {
	Courses        domain.CourseRepository
	Progress       domain.ProgressRepository
	Rewarder       usecase.Rewarder
	Premium        rest.Premium
	Guard          rest.Guard
	Clock          func() time.Time
	TargetLanguage func(context.Context, string) (string, error)
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Courses, d.Progress, d.Rewarder, d.Clock)
	return &Module{
		svc:     svc,
		handler: rest.NewHandler(svc, d.Premium, d.TargetLanguage),
		guard:   d.Guard,
	}
}

func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) {
	m.handler.Mount(mux, prefix, m.guard)
}
