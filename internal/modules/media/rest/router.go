package rest

import (
	"errors"
	"net/http"
	"path"

	"github.com/leksuu/platform/internal/modules/media/domain"
)

// Guard restricts routes (auth Authenticator satisfies it).
type Guard interface {
	Require(next http.Handler) http.Handler
	RequireRole(role string, next http.Handler) http.Handler
}

// Mount registers the admin-only upload endpoints and, for every store the API
// serves bytes for, the public file route at GET /media/{key}. A local-disk
// store reads from the filesystem; a private bucket is streamed through
// serveBlob. A bucket configured for anonymous public reads registers neither —
// its objects are fetched straight from the bucket or CDN.
func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard, store domain.Store) {
	mux.Handle("POST "+prefix+"/media", guard.RequireRole("admin", http.HandlerFunc(h.Upload)))
	mux.Handle("POST "+prefix+"/media/remote", guard.RequireRole("admin", http.HandlerFunc(h.ImportRemote)))
	mux.Handle("POST "+prefix+"/admin/course-import-sources", guard.RequireRole("admin", http.HandlerFunc(h.ArchiveCourseImport)))
	mux.Handle("POST "+prefix+"/media/avatar", guard.Require(http.HandlerFunc(h.UploadAvatar)))

	switch s := store.(type) {
	case domain.LocalDir:
		fs := http.StripPrefix("/media/", http.FileServer(http.Dir(s.Dir())))
		mux.Handle("GET /media/{file...}", longCache(fs))
	case domain.Reader:
		mux.Handle("GET /media/{file...}", longCache(serveBlob(s)))
	}
}

// serveBlob streams an object out of a private bucket. http.ServeContent does
// the HTTP work — Range, If-Modified-Since, 206 replies — so seeking inside a
// recording behaves exactly as it does for the local-disk store.
func serveBlob(rd domain.Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("file")
		if key == "" {
			http.NotFound(w, r)
			return
		}
		blob, err := rd.Get(r.Context(), key)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			// A credential or connectivity failure is the storage backend's
			// fault, not the caller's: 502, and never the bucket's error text.
			http.Error(w, "media unavailable", http.StatusBadGateway)
			return
		}
		defer blob.Body.Close()
		if blob.ContentType != "" {
			w.Header().Set("Content-Type", blob.ContentType)
		}
		http.ServeContent(w, r, path.Base(key), blob.ModTime, blob.Body)
	})
}

// longCache marks immutable, content-addressed media as cacheable for a year —
// the single biggest win for image/video load speed on repeat views.
func longCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}
