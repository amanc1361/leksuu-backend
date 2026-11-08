package rest

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/leksuu/platform/internal/modules/media/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type multipartInput struct {
	Ticket      string   `json:"ticket"`
	ContentType string   `json:"content_type"`
	Size        int64    `json:"size"`
	Licensed    bool     `json:"licensed"`
	PartNumber  int      `json:"part_number"`
	PartSHA256  []string `json:"part_sha256"`
}

func (h *Handler) decodeMultipart(w http.ResponseWriter, r *http.Request) (multipartInput, string, bool) {
	var in multipartInput
	principal, ok := authn.FromContext(r.Context())
	if !ok || principal.Subject == "" {
		web.Error(w, r, apperr.ErrInternal)
		return in, "", false
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return in, "", false
	}
	w.Header().Set("Cache-Control", "no-store")
	return in, principal.Subject, true
}

func multipartError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrMultipartTicket) || errors.Is(err, domain.ErrMultipartIncomplete) {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	web.Error(w, r, err)
}

func (h *Handler) BeginMultipart(w http.ResponseWriter, r *http.Request) {
	in, subject, ok := h.decodeMultipart(w, r)
	if !ok {
		return
	}
	ticket, partSize, err := h.multipart.BeginMultipart(r.Context(), subject, in.ContentType, in.Size, in.Licensed)
	if err != nil {
		multipartError(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{"ticket": ticket, "part_size": partSize})
}

func (h *Handler) MultipartPart(w http.ResponseWriter, r *http.Request) {
	in, subject, ok := h.decodeMultipart(w, r)
	if !ok {
		return
	}
	u, err := h.multipart.MultipartPartURL(r.Context(), in.Ticket, subject, in.PartNumber)
	if err != nil {
		multipartError(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"url": u})
}

func (h *Handler) MultipartStatus(w http.ResponseWriter, r *http.Request) {
	in, subject, ok := h.decodeMultipart(w, r)
	if !ok {
		return
	}
	parts, err := h.multipart.MultipartParts(r.Context(), in.Ticket, subject)
	if err != nil {
		multipartError(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"parts": parts})
}

func (h *Handler) CompleteMultipart(w http.ResponseWriter, r *http.Request) {
	in, subject, ok := h.decodeMultipart(w, r)
	if !ok {
		return
	}
	key, u, sha, err := h.multipart.CompleteMultipart(r.Context(), in.Ticket, subject, in.PartSHA256)
	if err != nil {
		multipartError(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{"key": key, "url": u, "sha256": sha})
}

func (h *Handler) AbortMultipart(w http.ResponseWriter, r *http.Request) {
	in, subject, ok := h.decodeMultipart(w, r)
	if !ok {
		return
	}
	if err := h.multipart.AbortMultipart(r.Context(), in.Ticket, subject); err != nil {
		multipartError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
