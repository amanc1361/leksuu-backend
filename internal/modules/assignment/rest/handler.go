// Package rest is the assignment delivery layer. Tutor-side routes create/list/
// grade; student-side routes list/view/submit. Ownership is enforced in the
// service, so every route just needs an authenticated principal.
package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/assignment/domain"
	"github.com/leksuu/platform/internal/modules/assignment/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// assignmentResp serializes an assignment together with its derived status.
type assignmentResp struct {
	*domain.Assignment
	Status string `json:"status"`
}

func resp(a *domain.Assignment) assignmentResp {
	return assignmentResp{Assignment: a, Status: a.Status()}
}

type Guard interface {
	Require(next http.Handler) http.Handler
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	// Tutor side.
	mux.Handle("POST "+prefix+"/tutor/assignments", guard.Require(http.HandlerFunc(h.Create)))
	mux.Handle("GET "+prefix+"/tutor/assignments", guard.Require(http.HandlerFunc(h.ListTutor)))
	mux.Handle("POST "+prefix+"/tutor/assignments/{id}/grade", guard.Require(http.HandlerFunc(h.Grade)))
	// Student side.
	mux.Handle("GET "+prefix+"/assignments", guard.Require(http.HandlerFunc(h.ListStudent)))
	mux.Handle("GET "+prefix+"/assignments/{id}", guard.Require(http.HandlerFunc(h.Get)))
	mux.Handle("POST "+prefix+"/assignments/{id}/submit", guard.Require(http.HandlerFunc(h.Submit)))
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
		StudentID string     `json:"student_id"`
		Type      string     `json:"type"`
		Title     string     `json:"title"`
		Prompt    string     `json:"prompt"`
		LessonID  string     `json:"lesson_id"`
		DueUTC    *time.Time `json:"due_utc"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	a, err := h.svc.Create(r.Context(), uid, usecase.CreateInput{
		StudentID: in.StudentID, Type: in.Type, Title: in.Title,
		Prompt: in.Prompt, LessonID: in.LessonID, DueUTC: in.DueUTC,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, resp(a))
}

func (h *Handler) ListTutor(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	vs, err := h.svc.ListForTutor(r.Context(), uid)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"assignments": vs})
}

func (h *Handler) ListStudent(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	vs, err := h.svc.ListForStudent(r.Context(), uid)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"assignments": vs})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	v, err := h.svc.Get(r.Context(), uid, r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, v)
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	a, err := h.svc.Submit(r.Context(), uid, r.PathValue("id"), in.Text)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, resp(a))
}

func (h *Handler) Grade(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Score    int    `json:"score"`
		Feedback string `json:"feedback"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	a, err := h.svc.Grade(r.Context(), uid, r.PathValue("id"), in.Score, in.Feedback)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, resp(a))
}
