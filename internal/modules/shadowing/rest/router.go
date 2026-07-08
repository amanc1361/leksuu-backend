package rest

import "net/http"

// Guard wraps a handler with authentication (auth Authenticator satisfies it).
type Guard interface {
	Require(next http.Handler) http.Handler
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("POST "+prefix+"/shadowing/rep", guard.Require(http.HandlerFunc(h.AddRep)))
	mux.Handle("GET "+prefix+"/shadowing/progress", guard.Require(http.HandlerFunc(h.Progress)))
}
