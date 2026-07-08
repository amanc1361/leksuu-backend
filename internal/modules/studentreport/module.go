// Package studentreport is the tutor-facing student progress aggregator facade.
package studentreport

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/studentreport/rest"
	"github.com/leksuu/platform/internal/modules/studentreport/usecase"
)

type Deps struct {
	Guard       rest.Guard
	Auth        usecase.TutorAuth
	Names       usecase.Names
	Game        usecase.Gamification
	SRS         usecase.SRS
	Pron        usecase.Pron
	Courses     usecase.Courses
	Assignments usecase.Assignments
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(usecase.Deps{
		Auth: d.Auth, Names: d.Names, Game: d.Game, SRS: d.SRS,
		Pron: d.Pron, Courses: d.Courses, Assignments: d.Assignments,
	})
	return &Module{handler: rest.NewHandler(svc), guard: d.Guard}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
