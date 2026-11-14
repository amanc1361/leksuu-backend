package rest

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

func (h *Handler) LearningRecords(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, domain.ErrUnauthenticated)
		return
	}
	records, err := h.svc.LearningRecords(r.Context(), p.Subject, r.PathValue("kind"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"records": records})
}

func (h *Handler) SaveLearningRecord(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, domain.ErrUnauthenticated)
		return
	}
	var record domain.LearningRecord
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12288))
	d.DisallowUnknownFields()
	if d.Decode(&record) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.SaveLearningRecord(r.Context(), p.Subject, r.PathValue("kind"), record); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
