// Package rest exposes shadowing progress: record a repetition and read per-line
// rep counts. Authenticated; progress is per user.
package rest

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/leksuu/platform/internal/modules/shadowing/domain"
	"github.com/leksuu/platform/internal/modules/shadowing/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func body(p *domain.Progress) map[string]any {
	return map[string]any{
		"item_id":         p.ItemID,
		"reps":            p.Reps,
		"activation_reps": domain.ActivationReps,
	}
}

func (h *Handler) AddRep(w http.ResponseWriter, r *http.Request) {
	pr, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		ItemID string `json:"item_id"`
		Line   int    `json:"line"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	p, err := h.svc.AddRep(r.Context(), pr.Subject, in.ItemID, strconv.Itoa(in.Line))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, body(p))
}

func (h *Handler) Progress(w http.ResponseWriter, r *http.Request) {
	pr, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	p, err := h.svc.Progress(r.Context(), pr.Subject, r.URL.Query().Get("item_id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, body(p))
}
