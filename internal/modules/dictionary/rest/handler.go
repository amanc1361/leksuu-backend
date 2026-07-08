// Package rest exposes admin endpoints to manage external dictionary providers
// and to look a word up through them.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/dictionary/domain"
	"github.com/leksuu/platform/internal/modules/dictionary/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func providerView(p domain.Provider) map[string]any {
	return map[string]any{
		"id": p.ID, "name": p.Name, "url": p.URL, "method": p.Method,
		"header_name": p.HeaderName, "header_value": p.HeaderValue,
		"body_template": p.BodyTemplate, "response_path": p.ResponsePath,
		"enabled": p.Enabled, "position": p.Position,
	}
}

type providerInput struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Method       string `json:"method"`
	HeaderName   string `json:"header_name"`
	HeaderValue  string `json:"header_value"`
	BodyTemplate string `json:"body_template"`
	ResponsePath string `json:"response_path"`
	Enabled      bool   `json:"enabled"`
	Position     int    `json:"position"`
}

func (in providerInput) toDomain(id string) *domain.Provider {
	return &domain.Provider{
		ID: id, Name: in.Name, URL: in.URL, Method: in.Method,
		HeaderName: in.HeaderName, HeaderValue: in.HeaderValue,
		BodyTemplate: in.BodyTemplate, ResponsePath: in.ResponsePath,
		Enabled: in.Enabled, Position: in.Position,
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ps, err := h.svc.List(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(ps))
	for _, p := range ps {
		out = append(out, providerView(p))
	}
	web.JSON(w, http.StatusOK, map[string]any{"providers": out})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in providerInput
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	p := in.toDomain("")
	if err := h.svc.Create(r.Context(), p); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, providerView(*p))
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var in providerInput
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	p := in.toDomain(r.PathValue("id"))
	if err := h.svc.Update(r.Context(), p); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, providerView(*p))
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Lookup(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Lookup(r.Context(), r.URL.Query().Get("word"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, res)
}
