package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/admin/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

func (h *Handler) ReviewMediaCue(w http.ResponseWriter, r *http.Request) {
	var in usecase.MediaCueReviewInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.ReviewMediaCue(r.Context(), r.PathValue("id"), in); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
