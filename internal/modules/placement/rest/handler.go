// Package rest exposes the placement test: fetch questions, submit answers,
// read the stored level.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/placement/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Guard interface {
	Require(next http.Handler) http.Handler
}

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("GET "+prefix+"/placement", guard.Require(http.HandlerFunc(h.Get)))
	mux.Handle("POST "+prefix+"/placement/submit", guard.Require(http.HandlerFunc(h.Submit)))
}

// Get returns the question bank (answer-free) plus the caller's stored level.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	res, err := h.svc.My(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"questions": h.svc.Questions(), "result": res})
}

// Submit grades and stores {answers: {questionID: optionIndex}}.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Answers map[string]int `json:"answers"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	res, err := h.svc.Submit(r.Context(), p.Subject, in.Answers)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, res)
}
