// Package notification is the in-app notification module facade: store + REST +
// live SSE, with a Notify port other modules call when events happen.
package notification

import (
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/notification/domain"
	"github.com/leksuu/platform/internal/modules/notification/rest"
	"github.com/leksuu/platform/internal/modules/notification/usecase"
)

type Deps struct {
	Repo   domain.Repository
	Guard  rest.Guard
	Tokens rest.TokenParser
	Clock  func() time.Time
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo, usecase.NewHub(), d.Clock)
	return &Module{svc: svc, handler: rest.NewHandler(svc, d.Tokens), guard: d.Guard}
}

// Service exposes the usecase so other modules can emit notifications.
func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
