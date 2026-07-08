package rest

import "net/http"

// Guard wraps a handler with authentication (auth Authenticator satisfies it).
type Guard interface {
	Require(next http.Handler) http.Handler
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("POST "+prefix+"/srs/cards", guard.Require(http.HandlerFunc(h.AddCard)))
	mux.Handle("GET "+prefix+"/srs/due", guard.Require(http.HandlerFunc(h.Due)))
	mux.Handle("POST "+prefix+"/srs/cards/{id}/review", guard.Require(http.HandlerFunc(h.Review)))
	mux.Handle("POST "+prefix+"/srs/cards/{id}/relearn", guard.Require(http.HandlerFunc(h.Relearn)))
	mux.Handle("POST "+prefix+"/srs/cards/{id}/undo", guard.Require(http.HandlerFunc(h.UndoReview)))
	mux.Handle("GET "+prefix+"/srs/cards", guard.Require(http.HandlerFunc(h.MyCards)))
	mux.Handle("POST "+prefix+"/srs/cards/{id}/suspend", guard.Require(http.HandlerFunc(h.SuspendCard)))
	mux.Handle("DELETE "+prefix+"/srs/cards/{id}", guard.Require(http.HandlerFunc(h.DeleteCard)))
	mux.Handle("GET "+prefix+"/srs/activity", guard.Require(http.HandlerFunc(h.Activity)))
	mux.Handle("GET "+prefix+"/srs/activity/week", guard.Require(http.HandlerFunc(h.WeekActivity)))
	mux.Handle("GET "+prefix+"/srs/settings", guard.Require(http.HandlerFunc(h.Settings)))
	mux.Handle("PUT "+prefix+"/srs/goal", guard.Require(http.HandlerFunc(h.SetGoal)))
	mux.Handle("GET "+prefix+"/srs/stats", guard.Require(http.HandlerFunc(h.Stats)))
	mux.Handle("GET "+prefix+"/srs/forecast", guard.Require(http.HandlerFunc(h.Forecast)))
	mux.Handle("GET "+prefix+"/srs/leeches", guard.Require(http.HandlerFunc(h.Leeches)))
	mux.Handle("GET "+prefix+"/vocab", guard.Require(http.HandlerFunc(h.ListVocab)))
	// public vocabulary lookup (click-word fallback) — no auth
	mux.HandleFunc("GET "+prefix+"/vocab/lookup", h.Lookup)
}
