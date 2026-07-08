package rest

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/media/domain"
)

// Guard restricts routes (auth Authenticator satisfies it).
type Guard interface {
	Require(next http.Handler) http.Handler
	RequireRole(role string, next http.Handler) http.Handler
}

// Mount registers the admin-only upload endpoint and, for local-disk stores, a
// cache-friendly public file route at GET /media/{file}.
func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard, store domain.Store) {
	mux.Handle("POST "+prefix+"/media", guard.RequireRole("admin", http.HandlerFunc(h.Upload)))
	mux.Handle("POST "+prefix+"/media/avatar", guard.Require(http.HandlerFunc(h.UploadAvatar)))

	if local, ok := store.(domain.LocalDir); ok {
		fs := http.StripPrefix("/media/", http.FileServer(http.Dir(local.Dir())))
		mux.Handle("GET /media/{file}", longCache(fs))
	}
}

// longCache marks immutable, content-addressed media as cacheable for a year —
// the single biggest win for image/video load speed on repeat views.
func longCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}
