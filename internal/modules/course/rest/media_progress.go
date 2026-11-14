package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

func (h *Handler) mediaProgressSubject(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if !learnerRequest(r) {
		web.Error(w, r, apperr.New("auth.learner_only", apperr.KindForbidden))
		return "", "", false
	}
	p, ok := authn.FromContext(r.Context())
	if !ok || p.Subject == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return "", "", false
	}
	if h.premium != nil {
		if err := h.premium.RequirePremium(r.Context(), p.Subject); err != nil {
			web.Error(w, r, err)
			return "", "", false
		}
	}
	lang := r.URL.Query().Get("lang")
	if h.targetLanguage != nil {
		var err error
		lang, err = h.targetLanguage(r.Context(), p.Subject)
		if err != nil {
			web.Error(w, r, err)
			return "", "", false
		}
	}
	if lang == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return "", "", false
	}
	return p.Subject, lang, true
}

func (h *Handler) GetMediaProgress(w http.ResponseWriter, r *http.Request) {
	user, lang, ok := h.mediaProgressSubject(w, r)
	if !ok {
		return
	}
	p, err := h.svc.MediaProgress(r.Context(), user, r.PathValue("id"), lang)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, p)
}

func (h *Handler) PutMediaProgress(w http.ResponseWriter, r *http.Request) {
	user, lang, ok := h.mediaProgressSubject(w, r)
	if !ok {
		return
	}
	var in struct {
		PositionSeconds float64 `json:"position_seconds"`
		Completed       bool    `json:"completed"`
		Revision        *int64  `json:"revision"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	revision, err := h.svc.SaveMediaPosition(r.Context(), user, r.PathValue("id"), lang, in.PositionSeconds, in.Completed, in.Revision)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]int64{"revision": revision})
}

func (h *Handler) PostMediaAttempt(w http.ResponseWriter, r *http.Request) {
	user, lang, ok := h.mediaProgressSubject(w, r)
	if !ok {
		return
	}
	var in struct {
		CueID          string `json:"cue_id"`
		Answer         string `json:"answer"`
		MissingIndices []int  `json:"missing_indices"`
		FullSentence   bool   `json:"full_sentence"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || in.CueID == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	p, passed, err := h.svc.RecordMediaAttempt(r.Context(), user, r.PathValue("id"), lang, in.CueID, in.Answer, in.MissingIndices, in.FullSentence)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"progress": p, "passed": passed})
}
