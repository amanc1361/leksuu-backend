package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/admin/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

func (h *Handler) ReviewMediaPublication(w http.ResponseWriter, r *http.Request) {
	var in usecase.MediaPublicationReviewInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.ReviewMediaPublication(r.Context(), r.PathValue("id"), in); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"updated": true})
}
