// Package tutor is the live-tutoring module facade: a marketplace of tutors with
// timezone-safe booking, reviews, and a video-room descriptor. The actual media
// transport (SFU: LiveKit/mediasoup + TURN) is an external integration; Room()
// returns a placeholder join token to wire it in.
package tutor

import (
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
	"github.com/leksuu/platform/internal/modules/tutor/rest"
	"github.com/leksuu/platform/internal/modules/tutor/usecase"
)

type Deps struct {
	Repo  domain.Repository
	Guard rest.Guard
	Clock func() time.Time
	Users usecase.UserDirectory
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
	svc     *usecase.Service
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo, d.Clock)
	svc.SetDirectory(d.Users)
	return &Module{handler: rest.NewHandler(svc), guard: d.Guard, svc: svc}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }

// Service exposes the usecase so other modules (e.g. rtc) can authorize room access.
func (m *Module) Service() *usecase.Service { return m.svc }
