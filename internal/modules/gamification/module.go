// Package gamification is the module facade for the motivation loop (XP, streak,
// league, daily goal). It exposes Mount for its read endpoint and Rewards() —
// the award entry point that other modules call through a port.
package gamification

import (
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/gamification/domain"
	"github.com/leksuu/platform/internal/modules/gamification/rest"
	"github.com/leksuu/platform/internal/modules/gamification/usecase"
)

type Deps struct {
	Repo  domain.Repository
	Guard rest.Guard
	Clock func() time.Time
}

type Module struct {
	svc     *usecase.Service
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	svc := usecase.NewService(d.Repo, d.Clock)
	return &Module{svc: svc, handler: rest.NewHandler(svc), guard: d.Guard}
}

func (m *Module) Service() *usecase.Service { return m.svc }

func (m *Module) Mount(mux *http.ServeMux, prefix string) {
	m.handler.Mount(mux, prefix, m.guard)
}

// Rewards returns the award entry point. Its Award(ctx, userID, xp, reason)
// signature satisfies the course module's Rewarder port (structural typing), so
// the composition root can wire rewards without the course module importing this one.
func (m *Module) Rewards() *usecase.Service { return m.svc }

// Wallet exposes gem Balance/Spend for the billing module (lesson unlocks).
func (m *Module) Wallet() *usecase.Service { return m.svc }
