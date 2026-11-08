package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/admin/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

func (h *Handler) EnqueueMediaJob(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ItemID         string `json:"item_id"`
		Operation      string `json:"operation"`
		TargetLanguage string `json:"target_language"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || in.ItemID == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	var job *usecase.MediaProcessingJob
	var err error
	switch in.Operation {
	case "", "synchronize":
		if in.TargetLanguage != "" {
			web.Error(w, r, apperr.ErrBadRequest)
			return
		}
		job, err = h.svc.EnqueueMediaJob(r.Context(), in.ItemID)
	case "translate":
		job, err = h.svc.EnqueueTranslationJob(r.Context(), in.ItemID, in.TargetLanguage)
	case "transcode":
		if in.TargetLanguage != "" {
			web.Error(w, r, apperr.ErrBadRequest)
			return
		}
		job, err = h.svc.EnqueueTranscodeJob(r.Context(), in.ItemID)
	default:
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusAccepted, job)
}

func (h *Handler) ListMediaJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.svc.ListMediaJobs(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (h *Handler) GetMediaJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.svc.MediaJob(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, job)
}

func (h *Handler) CancelMediaJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.svc.CancelMediaJob(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, job)
}

func (h *Handler) RetryMediaJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.svc.RetryMediaJob(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, job)
}

func (h *Handler) ApproveMediaJob(w http.ResponseWriter, r *http.Request) {
	job, err := h.svc.ApproveMediaJob(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, job)
}

func (h *Handler) ReviseMediaJobCue(w http.ResponseWriter, r *http.Request) {
	var in usecase.MediaJobCueRevision
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	job, err := h.svc.ReviseMediaJobCue(r.Context(), r.PathValue("id"), in)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, job)
}
