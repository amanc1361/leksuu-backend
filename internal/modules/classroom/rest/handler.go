// Package rest is the group-class delivery layer.
package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/classroom/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

type Guard interface {
	Require(next http.Handler) http.Handler
	RequireRole(role string, next http.Handler) http.Handler
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("POST "+prefix+"/tutor/group-classes", guard.Require(http.HandlerFunc(h.Create)))
	mux.Handle("GET "+prefix+"/tutor/group-classes", guard.Require(http.HandlerFunc(h.TutorClasses)))
	mux.Handle("GET "+prefix+"/group-classes", guard.Require(http.HandlerFunc(h.Browse)))
	mux.Handle("GET "+prefix+"/my/group-classes", guard.Require(http.HandlerFunc(h.MyClasses)))
	mux.Handle("POST "+prefix+"/group-classes/{id}/enroll", guard.Require(http.HandlerFunc(h.Enroll)))
	mux.Handle("POST "+prefix+"/group-classes/{id}/leave", guard.Require(http.HandlerFunc(h.Leave)))
	mux.Handle("GET "+prefix+"/group-classes/{id}/room", guard.Require(http.HandlerFunc(h.Room)))
	mux.Handle("GET "+prefix+"/admin/classes/stats", guard.RequireRole("admin", http.HandlerFunc(h.ClassStats)))
}

func sub(r *http.Request) (string, bool) {
	p, ok := authn.FromContext(r.Context())
	return p.Subject, ok
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Title       string    `json:"title"`
		Description string    `json:"description"`
		StartUTC    time.Time `json:"start_utc"`
		DurationMin int       `json:"duration_min"`
		Capacity    int       `json:"capacity"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	c, err := h.svc.Create(r.Context(), uid, usecase.CreateInput{
		Title: in.Title, Description: in.Description, StartUTC: in.StartUTC,
		DurationMin: in.DurationMin, Capacity: in.Capacity,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, c)
}

func (h *Handler) TutorClasses(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	vs, err := h.svc.TutorClasses(r.Context(), uid)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"classes": vs})
}

func (h *Handler) Browse(w http.ResponseWriter, r *http.Request) {
	uid, _ := sub(r)
	vs, err := h.svc.Browse(r.Context(), uid)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"classes": vs})
}

func (h *Handler) MyClasses(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	vs, err := h.svc.MyClasses(r.Context(), uid)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"classes": vs})
}

func (h *Handler) Enroll(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if err := h.svc.Enroll(r.Context(), uid, r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if err := h.svc.Leave(r.Context(), uid, r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Room returns join info (with the time-window state) for a group class.
func (h *Handler) Room(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	info, err := h.svc.Room(r.Context(), p.Subject, r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, info)
}

// ClassStats returns group-class counts for the admin overview (admin only).
func (h *Handler) ClassStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.ClassStats(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, stats)
}
