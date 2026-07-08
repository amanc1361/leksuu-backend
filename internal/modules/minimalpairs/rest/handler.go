package rest

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/minimalpairs/usecase"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

// Mount registers the public minimal-pairs content endpoints.
func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	mux.Handle("GET "+prefix+"/minimal-pairs", http.HandlerFunc(h.List))
	mux.Handle("GET "+prefix+"/minimal-pairs/{id}", http.HandlerFunc(h.Get))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.ListContrasts(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"contrasts": cs})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.Contrast(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, c)
}
