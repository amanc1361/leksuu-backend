// Package rest exposes pronunciation scoring for shadowing recordings.
// Authenticated learners POST an audio recording + the expected phrase.
package rest

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
	"github.com/leksuu/platform/internal/modules/pronunciation/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct {
	svc     *usecase.Service
	premium Premium // optional; nil = no premium gate
}

func NewHandler(svc *usecase.Service, premium Premium) *Handler {
	return &Handler{svc: svc, premium: premium}
}

const maxAudio = 8 << 20 // 8 MiB

func (h *Handler) Score(w http.ResponseWriter, r *http.Request) {
	pr, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if h.premium != nil && !pr.HasRole("admin") {
		if err := h.premium.RequirePremium(r.Context(), pr.Subject); err != nil {
			web.Error(w, r, err)
			return
		}
	}
	if err := r.ParseMultipartForm(maxAudio); err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	file, hdr, err := r.FormFile("audio")
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAudio))
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	line, _ := strconv.Atoi(r.FormValue("line"))
	a, ls, err := h.svc.Score(r.Context(), pr.Subject, r.FormValue("item_id"), line, domain.ScoreInput{
		Expected:    r.FormValue("expected"),
		Lang:        r.FormValue("lang"),
		Audio:       data,
		ContentType: hdr.Header.Get("Content-Type"),
	})
	if err != nil {
		if errors.Is(err, usecase.ErrScoringUnavailable) {
			web.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]string{"code": "pronunciation.unavailable", "message": "Pronunciation assessment is not configured."}})
			return
		}
		web.Error(w, r, err)
		return
	}
	resp := map[string]any{"assessment": a, "provider": h.svc.ProviderName()}
	if ls != nil {
		resp["line_score"] = ls
	}
	web.JSON(w, http.StatusOK, resp)
}

func (h *Handler) Capabilities(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, http.StatusOK, map[string]any{"available": h.svc.Available(), "provider": h.svc.ProviderName()})
}

func (h *Handler) Scores(w http.ResponseWriter, r *http.Request) {
	pr, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	scores, err := h.svc.Scores(r.Context(), pr.Subject, r.URL.Query().Get("item_id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"scores": scores})
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	pr, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	sum, err := h.svc.Summary(r.Context(), pr.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, sum)
}
