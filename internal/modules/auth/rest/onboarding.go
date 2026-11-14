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

func (h *Handler) GetOnboarding(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, domain.ErrUnauthenticated)
		return
	}
	state, err := h.svc.Onboarding(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, state)
}

func (h *Handler) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, domain.ErrUnauthenticated)
		return
	}
	var input struct {
		Answers domain.OnboardingAnswers `json:"answers"`
		Skipped bool                     `json:"skipped"`
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 8192))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	// Refuse an extra JSON value after the first object.
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	state, err := h.svc.CompleteOnboarding(r.Context(), p.Subject, input.Answers, input.Skipped)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, state)
}
