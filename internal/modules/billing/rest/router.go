package rest

import "net/http"

type Guard interface {
	Require(next http.Handler) http.Handler
	RequireRole(role string, next http.Handler) http.Handler
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("GET "+prefix+"/billing/plans", http.HandlerFunc(h.Plans))
	mux.Handle("GET "+prefix+"/billing/callback", http.HandlerFunc(h.Callback))
	mux.Handle("GET "+prefix+"/billing/me", guard.Require(http.HandlerFunc(h.Me)))
	mux.Handle("GET "+prefix+"/billing/class", guard.Require(http.HandlerFunc(h.ClassEntitlement)))
	mux.Handle("POST "+prefix+"/billing/subscribe", guard.Require(http.HandlerFunc(h.Subscribe)))
	mux.Handle("POST "+prefix+"/billing/consume", guard.Require(http.HandlerFunc(h.Consume)))
	mux.Handle("GET "+prefix+"/billing/lesson-access", guard.Require(http.HandlerFunc(h.LessonAccess)))
	mux.Handle("POST "+prefix+"/billing/unlock-lesson", guard.Require(http.HandlerFunc(h.UnlockLesson)))
	mux.Handle("GET "+prefix+"/admin/revenue", guard.RequireRole("admin", http.HandlerFunc(h.Revenue)))
	mux.Handle("POST "+prefix+"/admin/subscriptions/grant", guard.RequireRole("admin", http.HandlerFunc(h.GrantSubscription)))
	mux.Handle("GET "+prefix+"/admin/users/{id}/billing", guard.RequireRole("admin", http.HandlerFunc(h.UserBilling)))
}
