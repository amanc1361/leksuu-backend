// Package rest exposes the tutor-only student progress report.
package rest

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/studentreport/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

type Guard interface {
	Require(next http.Handler) http.Handler
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("GET "+prefix+"/tutor/students/{id}/progress", guard.Require(http.HandlerFunc(h.Progress)))
}

func (h *Handler) Progress(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	report, err := h.svc.Build(r.Context(), p.Subject, r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, report)
}
