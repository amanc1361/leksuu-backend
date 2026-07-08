// Package rest is the recording delivery layer. The chunk endpoint accepts a raw
// binary body (one webm fragment); the rest are small JSON calls.
package rest

import (
	"io"
	"net/http"

	"github.com/leksuu/platform/internal/modules/recording/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

// maxChunkBytes bounds a single uploaded fragment (16MB).
const maxChunkBytes = 16 << 20

type Guard interface {
	Require(next http.Handler) http.Handler
	RequireRole(role string, next http.Handler) http.Handler
}

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("POST "+prefix+"/classes/{classID}/recordings", guard.Require(http.HandlerFunc(h.Start)))
	mux.Handle("GET "+prefix+"/classes/{classID}/recordings", guard.Require(http.HandlerFunc(h.List)))
	mux.Handle("POST "+prefix+"/recordings/{recID}/chunk", guard.Require(http.HandlerFunc(h.Chunk)))
	mux.Handle("POST "+prefix+"/recordings/{recID}/complete", guard.Require(http.HandlerFunc(h.Complete)))
	mux.Handle("GET "+prefix+"/admin/recordings", guard.RequireRole("admin", http.HandlerFunc(h.AdminList)))
	mux.Handle("DELETE "+prefix+"/admin/recordings/{recID}", guard.RequireRole("admin", http.HandlerFunc(h.AdminDelete)))
	mux.Handle("POST "+prefix+"/admin/recordings/purge", guard.RequireRole("admin", http.HandlerFunc(h.AdminPurge)))
}

func sub(r *http.Request) (string, bool) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		return "", false
	}
	return p.Subject, true
}

func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	rec, err := h.svc.Start(r.Context(), r.PathValue("classID"), uid)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, rec)
}

func (h *Handler) Chunk(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxChunkBytes+1))
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if len(data) == 0 || len(data) > maxChunkBytes {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	size, err := h.svc.AppendChunk(r.Context(), r.PathValue("recID"), uid, data)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"size_bytes": size})
}

func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	rec, err := h.svc.Complete(r.Context(), r.PathValue("recID"), uid)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, rec)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	uid, ok := sub(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	recs, err := h.svc.ListForClass(r.Context(), r.PathValue("classID"), uid)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"recordings": recs})
}

// AdminList returns every recording plus the total storage used (admin only).
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	recs, err := h.svc.ListAll(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	var total int64
	for _, rec := range recs {
		total += rec.SizeBytes
	}
	web.JSON(w, http.StatusOK, map[string]any{"recordings": recs, "total_bytes": total, "count": len(recs), "retention_days": h.svc.RetentionDays()})
}

// AdminDelete removes a recording's object + metadata to free storage (admin only).
func (h *Handler) AdminDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRecording(r.Context(), r.PathValue("recID")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// AdminPurge runs the retention cleanup on demand and reports how many were
// removed (admin only). When retention is disabled the count is zero.
func (h *Handler) AdminPurge(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.PurgeExpired(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"deleted": n, "retention_days": h.svc.RetentionDays()})
}
