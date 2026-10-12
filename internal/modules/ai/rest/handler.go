// Package rest exposes the admin AI-authoring endpoint: request a content draft.
package rest

import (
	"context"
	"encoding/json"
	"github.com/leksuu/platform/internal/shared/authn"
	"net/http"

	"github.com/leksuu/platform/internal/modules/ai/domain"
	"github.com/leksuu/platform/internal/modules/ai/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct {
	svc     *usecase.Service
	premium Premium
}

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Premium checks the learning subscription. The AI coach is not part of the
// free tier: every message costs a model call.
type Premium interface {
	RequirePremium(ctx context.Context, userID string) error
}

// SetPremium enables the subscription check on the coach.
func (h *Handler) SetPremium(p Premium) { h.premium = p }

// requirePremium writes the error and returns false when the caller has no
// learning subscription.
func (h *Handler) requirePremium(w http.ResponseWriter, r *http.Request) bool {
	if h.premium == nil {
		return true
	}
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return false
	}
	if err := h.premium.RequirePremium(r.Context(), p.Subject); err != nil {
		web.Error(w, r, err)
		return false
	}
	return true
}

type Guard interface {
	RequireRole(role string, next http.Handler) http.Handler
	Require(next http.Handler) http.Handler
}

const roleAdmin = "admin"

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("POST "+prefix+"/admin/ai/draft", guard.RequireRole(roleAdmin, http.HandlerFunc(h.Draft)))
	mux.Handle("POST "+prefix+"/admin/ai/vocab", guard.RequireRole(roleAdmin, http.HandlerFunc(h.Vocab)))
	mux.Handle("POST "+prefix+"/admin/ai/translate", guard.RequireRole(roleAdmin, http.HandlerFunc(h.Translate)))
	// Learner-facing AI coach (Phase 3): writing correction + level-limited chat.
	mux.Handle("POST "+prefix+"/ai/writing", guard.Require(http.HandlerFunc(h.Writing)))
	mux.Handle("POST "+prefix+"/ai/chat", guard.Require(http.HandlerFunc(h.Chat)))
}

func (h *Handler) Draft(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Topic      string `json:"topic"`
		Level      string `json:"level"`
		TargetLang string `json:"target_lang"`
		NativeLang string `json:"native_lang"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	d, err := h.svc.Draft(r.Context(), domain.DraftInput{
		Topic: in.Topic, Level: in.Level, TargetLang: in.TargetLang, NativeLang: in.NativeLang,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"draft": d, "provider": h.svc.ProviderName()})
}

func (h *Handler) Vocab(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TargetLang string `json:"target_lang"`
		NativeLang string `json:"native_lang"`
		Level      string `json:"level"`
		Topic      string `json:"topic"`
		Count      int    `json:"count"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	items, err := h.svc.DraftVocab(r.Context(), domain.VocabInput{
		TargetLang: in.TargetLang, NativeLang: in.NativeLang, Level: in.Level, Topic: in.Topic, Count: in.Count,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"items": items, "provider": h.svc.ProviderName()})
}

func (h *Handler) Translate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text     string `json:"word"`
		FromLang string `json:"from_lang"`
		ToLang   string `json:"to_lang"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	out, err := h.svc.Translate(r.Context(), domain.TranslateInput{Text: in.Text, FromLang: in.FromLang, ToLang: in.ToLang})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"translation": out, "provider": h.svc.ProviderName()})
}

func (h *Handler) Writing(w http.ResponseWriter, r *http.Request) {
	if !h.requirePremium(w, r) {
		return
	}
	var in struct {
		Text       string `json:"text"`
		TargetLang string `json:"target_lang"`
		NativeLang string `json:"native_lang"`
		Level      string `json:"level"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	fb, err := h.svc.WritingFeedback(r.Context(), domain.WritingInput{
		Text: in.Text, TargetLang: in.TargetLang, NativeLang: in.NativeLang, Level: in.Level,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"feedback": fb, "provider": h.svc.ProviderName()})
}

func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	if !h.requirePremium(w, r) {
		return
	}
	var in struct {
		Messages   []domain.ChatMessage `json:"messages"`
		TargetLang string               `json:"target_lang"`
		NativeLang string               `json:"native_lang"`
		Level      string               `json:"level"`
		Scenario   *domain.ChatScenario `json:"scenario"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	rep, err := h.svc.Chat(r.Context(), domain.ChatInput{
		Messages: in.Messages, TargetLang: in.TargetLang, NativeLang: in.NativeLang, Level: in.Level, Scenario: in.Scenario,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"reply": rep, "provider": h.svc.ProviderName()})
}
