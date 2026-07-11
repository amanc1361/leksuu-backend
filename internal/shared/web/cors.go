package web

import (
	"net/http"
	"strings"
)

// CORS wraps a handler with cross-origin headers for the SPA frontend. The
// allowed origin is configured (no wildcard with credentials). Preflight
// OPTIONS requests are answered here.
func CORS(allowedOrigin string, next http.Handler) http.Handler {
	allowedOrigins := parseAllowedOrigins(allowedOrigin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowedOrigins["*"]; ok {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language")
				w.Header().Set("Access-Control-Max-Age", "600")
			} else if _, ok := allowedOrigins[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseAllowedOrigins(allowedOrigin string) map[string]struct{} {
	allowedOrigins := make(map[string]struct{})
	for _, origin := range strings.Split(allowedOrigin, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowedOrigins[origin] = struct{}{}
		}
	}
	return allowedOrigins
}
