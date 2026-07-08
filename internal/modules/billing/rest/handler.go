// Package rest exposes monetization endpoints: plan catalog, entitlements,
// subscribe (redirect to gateway), the gateway callback, and quota consume.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/billing/domain"
	"github.com/leksuu/platform/internal/modules/billing/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Plans(w http.ResponseWriter, r *http.Request) {
	product := r.URL.Query().Get("product")
	if product == "" {
		product = "courses"
	}
	web.JSON(w, http.StatusOK, map[string]any{"plans": h.svc.Plans(r.Context(), product), "product": product, "provider": h.svc.ProviderName()})
}

// ClassEntitlement reports the user's class-subscription status.
func (h *Handler) ClassEntitlement(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	ent, err := h.svc.Entitlement(r.Context(), p.Subject, "classes")
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, ent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	ent, err := h.svc.Entitlements(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, ent)
}

func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var body struct {
		Product string `json:"product"`
		Period  string `json:"period"`
		Plan    string `json:"plan"` // legacy alias for period
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	period := body.Period
	if period == "" {
		period = body.Plan
	}
	payURL, err := h.svc.Subscribe(r.Context(), p.Subject, body.Product, period)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]string{"pay_url": payURL})
}

func (h *Handler) Consume(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ent, err := h.svc.Consume(r.Context(), p.Subject, body.Action)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, ent)
}

func (h *Handler) LessonAccess(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	d, err := h.svc.LessonAccess(r.Context(), p.Subject, r.URL.Query().Get("lesson_id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, d)
}

func (h *Handler) UnlockLesson(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var body struct {
		LessonID string `json:"lesson_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	d, err := h.svc.UnlockLesson(r.Context(), p.Subject, body.LessonID)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, d)
}

// Callback is hit by the payment gateway (public). It verifies and redirects the
// user's browser to the frontend success/failure page.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	authority := q.Get("Authority")
	if authority == "" {
		authority = q.Get("authority")
	}
	status := q.Get("Status")
	if status == "" {
		status = q.Get("status")
	}
	redirect := h.svc.HandleCallback(r.Context(), authority, status)
	http.Redirect(w, r, redirect, http.StatusFound)
}

var _ = domain.PlanFree

// Revenue returns the platform subscription-income summary (admin only).
func (h *Handler) Revenue(w http.ResponseWriter, r *http.Request) {
	sum, err := h.svc.RevenueSummary(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, sum)
}

// GrantSubscription comps a user a subscription (admin only).
func (h *Handler) GrantSubscription(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID  string `json:"user_id"`
		Product string `json:"product"`
		Days    int    `json:"days"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.UserID == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.GrantSubscription(r.Context(), in.UserID, in.Product, in.Days); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// UserBilling returns a user's subscriptions + payment history (admin only).
func (h *Handler) UserBilling(w http.ResponseWriter, r *http.Request) {
	detail, err := h.svc.UserBillingDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, detail)
}
