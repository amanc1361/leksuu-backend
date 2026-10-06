package web

import (
	"net/http"
	"strings"
)

// CORS wraps a handler with cross-origin headers for the SPA frontend. Multiple
// origins are supported (e.g. the production domain plus localhost); an origin
// matches when it appears in allowedOrigins, or when "*" is present. Matching is
// case-insensitive. Preflight OPTIONS requests are answered here.
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	allowAll := false
	set := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(strings.ToLower(o))
		if o == "*" {
			allowAll = true
		} else if o != "" {
			set[o] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		_, ok := set[strings.ToLower(origin)]
		if origin != "" && (allowAll || ok) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
