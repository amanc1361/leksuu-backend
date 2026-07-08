// Package rest is the notification delivery layer: a REST API for listing and
// reading notifications, plus a live Server-Sent Events stream for the bell.
package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/leksuu/platform/internal/modules/notification/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Guard interface {
	Require(next http.Handler) http.Handler
}

// TokenParser validates a raw JWT (used for the SSE query-token, since the
// browser EventSource API cannot send Authorization headers).
type TokenParser interface {
	ParseToken(raw string) (authn.Principal, error)
}

type Handler struct {
	svc    *usecase.Service
	tokens TokenParser
}

func NewHandler(svc *usecase.Service, tokens TokenParser) *Handler {
	return &Handler{svc: svc, tokens: tokens}
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("GET "+prefix+"/notifications", guard.Require(http.HandlerFunc(h.List)))
	mux.Handle("GET "+prefix+"/notifications/unread-count", guard.Require(http.HandlerFunc(h.Unread)))
	mux.Handle("POST "+prefix+"/notifications/read-all", guard.Require(http.HandlerFunc(h.ReadAll)))
	mux.Handle("POST "+prefix+"/notifications/{id}/read", guard.Require(http.HandlerFunc(h.Read)))
	mux.Handle("GET "+prefix+"/notifications/settings", guard.Require(http.HandlerFunc(h.GetSettings)))
	mux.Handle("PUT "+prefix+"/notifications/settings", guard.Require(http.HandlerFunc(h.PutSettings)))
	// SSE stream uses ?token= query auth (EventSource can't set headers).
	mux.HandleFunc("GET "+prefix+"/notifications/stream", h.Stream)
}

func uid(r *http.Request) (string, bool) {
	p, ok := authn.FromContext(r.Context())
	return p.Subject, ok
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	ns, err := h.svc.List(r.Context(), u, limit, offset)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	unread, _ := h.svc.UnreadCount(r.Context(), u)
	web.JSON(w, http.StatusOK, map[string]any{"notifications": ns, "unread": unread})
}

func (h *Handler) Unread(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	n, err := h.svc.UnreadCount(r.Context(), u)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"unread": n})
}

func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if err := h.svc.MarkRead(r.Context(), u, r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) ReadAll(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if err := h.svc.MarkAllRead(r.Context(), u); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GetSettings returns the user's per-type notification preferences.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	prefs, err := h.svc.Prefs(r.Context(), u)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"settings": prefs})
}

// PutSettings turns a single notification type on or off for the user.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Type == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.SetPref(r.Context(), u, in.Type, in.Enabled); err != nil {
		web.Error(w, r, err)
		return
	}
	prefs, _ := h.svc.Prefs(r.Context(), u)
	web.JSON(w, http.StatusOK, map[string]any{"settings": prefs})
}

// Stream is a Server-Sent Events endpoint pushing live notifications.
func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	principal, err := h.tokens.ParseToken(r.URL.Query().Get("token"))
	if err != nil {
		web.Error(w, r, apperr.New("auth.unauthorized", apperr.KindUnauthorized))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)

	ch, cancel := h.svc.Hub().Subscribe(principal.Subject)
	defer cancel()

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case n := <-ch:
			if n == nil {
				return
			}
			b, _ := json.Marshal(n)
			fmt.Fprintf(w, "event: notification\ndata: %s\n\n", b)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
