package rest

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/leksuu/platform/internal/modules/media/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct {
	svc      *usecase.Service
	maxBytes int64
	// avatarGate answers "may this user upload an avatar?" — wired to the
	// tutor module (approved teachers) after construction.
	avatarGate func(ctx context.Context, userID string) bool
}

// SetAvatarGate wires the late-bound uploader check.
func (h *Handler) SetAvatarGate(f func(ctx context.Context, userID string) bool) { h.avatarGate = f }

func NewHandler(svc *usecase.Service, maxBytes int64) *Handler {
	return &Handler{svc: svc, maxBytes: maxBytes}
}

// Upload accepts a multipart "file" field and stores it in object storage.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	up, err := h.svc.Upload(r.Context(), hdr.Filename, ct, data)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, up)
}

// avatarMaxBytes keeps avatars small (they render at ≤128px anywhere).
const avatarMaxBytes = 2 << 20

// UploadAvatar accepts an image for the caller's public listing. Gated to
// approved teachers via the avatar gate; images only, 2MB cap.
func (h *Handler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if h.avatarGate == nil || !h.avatarGate(r.Context(), p.Subject) {
		web.Error(w, r, apperr.New("media.not_allowed", apperr.KindForbidden))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, avatarMaxBytes+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if len(data) > avatarMaxBytes {
		web.Error(w, r, apperr.New("media.too_large", apperr.KindValidation))
		return
	}
	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(ct), "image/") {
		web.Error(w, r, apperr.New("media.bad_type", apperr.KindValidation))
		return
	}
	up, err := h.svc.Upload(r.Context(), hdr.Filename, ct, data)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, up)
}
