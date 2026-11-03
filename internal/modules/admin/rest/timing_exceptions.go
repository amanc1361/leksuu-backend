package rest

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

func (h *Handler) IngestTimingExceptions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
	if err != nil || !json.Valid(body) {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	added, total, err := h.svc.IngestTimingExceptionReport(r.Context(), body)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]int{"added": added, "total": total})
}

func (h *Handler) ListTimingExceptions(w http.ResponseWriter, r *http.Request) {
	records, err := h.svc.TimingExceptionQueue(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"schema": "leksuu.exception-review.v2", "records": records})
}
