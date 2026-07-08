// Package rest exposes the admin settings endpoints.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/settings/domain"
	"github.com/leksuu/platform/internal/modules/settings/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

type Guard interface {
	RequireRole(role string, next http.Handler) http.Handler
}

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("GET "+prefix+"/admin/settings", guard.RequireRole("admin", http.HandlerFunc(h.Get)))
	mux.Handle("PUT "+prefix+"/admin/settings", guard.RequireRole("admin", http.HandlerFunc(h.Update)))
	// Dynamic subscription plans (admin-managed).
	mux.Handle("GET "+prefix+"/admin/plans", guard.RequireRole("admin", http.HandlerFunc(h.ListPlans)))
	mux.Handle("POST "+prefix+"/admin/plans", guard.RequireRole("admin", http.HandlerFunc(h.UpsertPlan)))
	mux.Handle("DELETE "+prefix+"/admin/plans/{product}/{id}", guard.RequireRole("admin", http.HandlerFunc(h.DeletePlan)))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Get(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, s)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var in domain.Settings
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	s, err := h.svc.Update(r.Context(), in)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, s)
}

// ListPlans returns every subscription plan (admin management view).
func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.svc.PlansAll(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	if plans == nil {
		plans = []domain.Plan{}
	}
	web.JSON(w, http.StatusOK, map[string]any{"plans": plans})
}

// UpsertPlan creates or updates a plan (keyed by product + id).
func (h *Handler) UpsertPlan(w http.ResponseWriter, r *http.Request) {
	var in domain.Plan
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	in.Active = true // a saved plan is active unless explicitly toggled off in a future iteration
	if err := h.svc.UpsertPlan(r.Context(), in); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, in)
}

// DeletePlan removes a plan by product + id.
func (h *Handler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	product := r.PathValue("product")
	id := r.PathValue("id")
	if err := h.svc.DeletePlan(r.Context(), product, id); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
