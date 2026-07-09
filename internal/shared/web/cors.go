package web

import (
	"net/http"
	"strings"
)

// CORS wraps a handler with cross-origin headers for the SPA frontend. The
// allowed origins are configured as a comma-separated list. Preflight OPTIONS
// requests are answered here.
func CORS(allowedOrigins string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{})
	allowAny := false
	for _, origin := range strings.Split(allowedOrigins, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			if origin == "*" {
				allowAny = true
			}
			allowed[origin] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		_, originAllowed := allowed[origin]
		if origin != "" && (allowAny || originAllowed) {
			if allowAny {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
