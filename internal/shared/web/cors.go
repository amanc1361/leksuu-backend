package web

import (
	"net/http"
	"strings"
)

// CORS wraps a handler with cross-origin headers for the SPA frontend. Multiple
// origins are supported (e.g. the production domain plus localhost); matching
// is case-insensitive. A listed origin is echoed back and may send credentials.
// "*" admits any other origin too, but as a public wildcard: the response says
// "*" and never allows credentials (browsers reject that combination, and it
// would let any site act with a user's cookies). Preflight OPTIONS requests are
// answered here.
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
		_, listed := set[strings.ToLower(origin)]
		if origin != "" && (listed || allowAll) {
			h := w.Header()
			if listed {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
			} else {
				h.Set("Access-Control-Allow-Origin", "*")
			}
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept-Language")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
