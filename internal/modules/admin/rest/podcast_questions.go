package rest

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/leksuu/platform/internal/modules/admin/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

func (h *Handler) SavePodcastQuestions(w http.ResponseWriter, r *http.Request) {
	var input usecase.PodcastQuestionsInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.SavePodcastQuestions(r.Context(), r.PathValue("id"), input); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
