package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/admin/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

func (h *Handler) ReviseStoryTranslation(w http.ResponseWriter, r *http.Request) {
	var in usecase.StoryTranslationRevisionInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	result, err := h.svc.ReviseStoryTranslation(r.Context(), r.PathValue("sourceID"), in)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, result)
}

func (h *Handler) ApproveStoryTranslations(w http.ResponseWriter, r *http.Request) {
	var in usecase.StoryTranslationApprovalInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	result, err := h.svc.ApproveStoryTranslations(r.Context(), r.PathValue("sourceID"), in)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, result)
}

func (h *Handler) DecideStoryTranslation(w http.ResponseWriter, r *http.Request) {
	var in usecase.StoryTranslationDecisionInput
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	result, err := h.svc.DecideStoryTranslation(r.Context(), r.PathValue("sourceID"), in)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, result)
}
