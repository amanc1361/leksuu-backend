package rest

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"

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
	mux.Handle("GET "+prefix+"/admin/media-preview", guard.RequireRole("admin", http.HandlerFunc(h.AdminPreviewGrant)))
	if h.multipart != nil {
		mux.Handle("POST "+prefix+"/media/multipart", guard.RequireRole("admin", http.HandlerFunc(h.BeginMultipart)))
		mux.Handle("POST "+prefix+"/media/multipart/part", guard.RequireRole("admin", http.HandlerFunc(h.MultipartPart)))
		mux.Handle("POST "+prefix+"/media/multipart/status", guard.RequireRole("admin", http.HandlerFunc(h.MultipartStatus)))
		mux.Handle("POST "+prefix+"/media/multipart/complete", guard.RequireRole("admin", http.HandlerFunc(h.CompleteMultipart)))
		mux.Handle("POST "+prefix+"/media/multipart/abort", guard.RequireRole("admin", http.HandlerFunc(h.AbortMultipart)))
	}
	mux.Handle("POST "+prefix+"/media/remote", guard.RequireRole("admin", http.HandlerFunc(h.ImportRemote)))
	mux.Handle("POST "+prefix+"/admin/course-import-sources", guard.RequireRole("admin", http.HandlerFunc(h.ArchiveCourseImport)))
	mux.Handle("POST "+prefix+"/media/avatar", guard.Require(http.HandlerFunc(h.UploadAvatar)))

	// Playback credentials are issued only after the learner's entitlement has
	// been checked. The same credential is required on every HLS child request.
	mux.Handle("GET "+prefix+"/media-playback/{item}", guard.Require(http.HandlerFunc(h.PlaybackGrant)))
	switch s := store.(type) {
	case domain.LocalDir:
		fs := http.StripPrefix("/media/", http.FileServer(http.Dir(s.Dir())))
		mux.Handle("GET /media/{file...}", inert(longCache(h.authorizeBytes(fs))))
	case domain.Reader:
		mux.Handle("GET /media/{file...}", inert(longCache(h.authorizeBytes(serveBlob(s)))))
	}
}

// KeyOwnership resolves existing Film/Music assets, including old random keys.
// Unknown keys under protected storage prefixes fail closed.
type KeyOwnership func(context.Context, string) (bool, error)

func (h *Handler) authorizeBytes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("file")
		protected := strings.HasPrefix(key, "hls/") || strings.HasPrefix(key, "protected/")
		if !protected && h.keyOwnership != nil {
			owned, err := h.keyOwnership(r.Context(), key)
			if err != nil {
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "media unavailable", http.StatusServiceUnavailable)
				return
			}
			protected = protected || owned
		}
		if protected {
			if !h.validPlaybackTicket(r.URL.Query().Get("playback"), key) {
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "playback authorization required", http.StatusForbidden)
				return
			}
			if h.playbackCDN && strings.HasSuffix(strings.ToLower(key), ".ts") {
				w.Header().Set("Cache-Control", "public, max-age=0, s-maxage="+strconv.Itoa(playbackCacheSeconds(r.URL.Query().Get("playback"))))
			} else {
				w.Header().Set("Cache-Control", "private, max-age=0, no-store")
			}
			if strings.HasSuffix(strings.ToLower(key), ".m3u8") {
				h.servePlaylist(w, r, key)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
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

// inert keeps stored files from ever acting as part of the site: the browser
// must trust the served content type (nosniff), and a file opened directly
// renders in a sandbox with no script and an opaque origin, so even an HTML or
// SVG document stored by an older version can never read the app's session.
// Neither header affects images, audio or video embedded in a page.
func inert(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox")
		next.ServeHTTP(w, r)
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
