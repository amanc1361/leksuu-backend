package rest

import "net/http"

// Guard restricts routes to a role (auth Authenticator satisfies it).
type Guard interface {
	RequireRole(role string, next http.Handler) http.Handler
}

const roleAdmin = "admin"

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	admin := func(fn http.HandlerFunc) http.Handler { return guard.RequireRole(roleAdmin, fn) }
	mux.Handle("GET "+prefix+"/admin/dict-providers", admin(h.List))
	mux.Handle("POST "+prefix+"/admin/dict-providers", admin(h.Create))
	mux.Handle("PATCH "+prefix+"/admin/dict-providers/{id}", admin(h.Update))
	mux.Handle("DELETE "+prefix+"/admin/dict-providers/{id}", admin(h.Delete))
	mux.Handle("GET "+prefix+"/admin/dict-lookup", admin(h.Lookup))
}
