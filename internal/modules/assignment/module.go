// Package assignment is the homework module facade: tutors assign work, students
// submit responses, tutors grade. Reuses the auth user directory for names and
// the tutor module to gate who may assign.
package assignment

import (
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/assignment/domain"
	"github.com/leksuu/platform/internal/modules/assignment/rest"
	"github.com/leksuu/platform/internal/modules/assignment/usecase"
)

type Deps struct {
	Repo   domain.Repository
	Guard  rest.Guard
	Tutors usecase.TutorCheck
	Users  usecase.UserDirectory
	Clock  func() time.Time
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo, d.Tutors, d.Users, d.Clock)
	return &Module{svc: svc, handler: rest.NewHandler(svc), guard: d.Guard}
}

func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
