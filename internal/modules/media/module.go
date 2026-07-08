// Package media is the module facade for the media (object-storage) bounded
// context. It owns uploads and public delivery; the Store port lets the
// composition root pick local disk or an S3-compatible backend.
package media

import (
	"context"
	"net/http"

	"github.com/leksuu/platform/internal/modules/media/domain"
	"github.com/leksuu/platform/internal/modules/media/rest"
	"github.com/leksuu/platform/internal/modules/media/usecase"
)

type Deps struct {
	Store    domain.Store
	Guard    rest.Guard
	MaxBytes int64
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
	store   domain.Store
}

func New(d Deps) *Module {
	return &Module{
		handler: rest.NewHandler(usecase.NewService(d.Store, d.MaxBytes), d.MaxBytes),
		guard:   d.Guard,
		store:   d.Store,
	}
}

// SetAvatarGate wires the "may upload an avatar" check (tutor module).
func (m *Module) SetAvatarGate(f func(ctx context.Context, userID string) bool) {
	m.handler.SetAvatarGate(f)
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) {
	m.handler.Mount(mux, prefix, m.guard, m.store)
}
