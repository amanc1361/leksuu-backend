// Package classroom is the group-class module facade: tutors schedule shared
// rooms, students enroll, and CanJoin authorizes the live mesh.
package classroom

import (
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/classroom/domain"
	"github.com/leksuu/platform/internal/modules/classroom/rest"
	"github.com/leksuu/platform/internal/modules/classroom/usecase"
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

// Service exposes the usecase so the rtc authorizer can authorize class rooms.
func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
