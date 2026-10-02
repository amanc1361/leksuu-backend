// Package rest exposes the admin AI-authoring endpoint: request a content draft.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/ai/domain"
	"github.com/leksuu/platform/internal/modules/ai/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

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
