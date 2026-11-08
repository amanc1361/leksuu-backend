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
	Store          domain.Store
	Guard          rest.Guard
	MaxBytes       int64
	PlaybackSecret []byte
	PlaybackBase   string
	PlaybackCDN    bool
	PlaybackAccess func(context.Context, string, string, bool) (rest.PlaybackAsset, error)
	KeyOwnership   rest.KeyOwnership
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
	store   domain.Store
}

func New(d Deps) *Module {
	handler := rest.NewHandler(usecase.NewService(d.Store, d.MaxBytes), d.MaxBytes)
	if multipart, ok := d.Store.(domain.MultipartStore); ok {
		handler.SetMultipartStore(multipart)
	}
	handler.ConfigurePlayback(d.PlaybackSecret, d.PlaybackBase, d.PlaybackCDN, d.PlaybackAccess, d.KeyOwnership, d.Store)
	return &Module{
		handler: handler,
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
