package rest

import (
	"context"
	"net/http"
)

type Guard interface {
	Require(next http.Handler) http.Handler
}

// Premium gates a handler behind an active subscription (billing satisfies it).
type Premium interface {
	RequirePremium(ctx context.Context, userID string) error
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("POST "+prefix+"/pronunciation/score", guard.Require(http.HandlerFunc(h.Score)))
	mux.Handle("GET "+prefix+"/pronunciation/scores", guard.Require(http.HandlerFunc(h.Scores)))
	mux.Handle("GET "+prefix+"/pronunciation/summary", guard.Require(http.HandlerFunc(h.Summary)))
}
