package rest

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/modules/auth/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

// Handler adapts HTTP requests to the auth use case.
type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func decode(r *http.Request, v any) bool {
	return json.NewDecoder(r.Body).Decode(v) == nil
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password, DisplayName, NativeLang string }
	if !decode(r, &in) {
		web.Error(w, r, domain.ErrEmailRequired)
		return
	}
	u, err := h.svc.Register(r.Context(), usecase.RegisterInput{
		Email: in.Email, Password: in.Password, DisplayName: in.DisplayName, NativeLang: in.NativeLang,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{
		"id": u.ID, "email": u.Email, "display_name": u.DisplayName, "native_lang": u.NativeLang,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password, DeviceID string }
	if !decode(r, &in) {
		web.Error(w, r, domain.ErrInvalidCredentials)
		return
	}
	pair, err := h.svc.Login(r.Context(), in.Email, in.Password, in.DeviceID)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	writeTokens(w, pair)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var in struct{ RefreshToken, DeviceID string }
	if !decode(r, &in) || in.RefreshToken == "" {
		web.Error(w, r, domain.ErrRefreshRequired)
		return
	}
	pair, err := h.svc.Refresh(r.Context(), in.RefreshToken, in.DeviceID)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	writeTokens(w, pair)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var in struct{ RefreshToken string }
	_ = decode(r, &in)
	_ = h.svc.Logout(r.Context(), in.RefreshToken)
	web.JSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, domain.ErrUnauthenticated)
		return
	}
	u, err := h.svc.Profile(r.Context(), principal.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{
		"id": u.ID, "email": u.Email, "display_name": u.DisplayName,
		"native_lang": u.NativeLang, "target_lang": u.TargetLang,
		"cefr": u.CEFR, "roles": principal.Roles,
	})
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, domain.ErrUnauthenticated)
		return
	}
	var in struct {
		DisplayName string `json:"display_name"`
		NativeLang  string `json:"native_lang"`
		TargetLang  string `json:"target_lang"`
		CEFR        string `json:"cefr"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	u, err := h.svc.UpdateProfile(r.Context(), principal.Subject, usecase.UpdateProfileInput(in))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{
		"id": u.ID, "email": u.Email, "display_name": u.DisplayName,
		"native_lang": u.NativeLang, "target_lang": u.TargetLang,
		"cefr": u.CEFR, "roles": principal.Roles,
	})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, domain.ErrUnauthenticated)
		return
	}
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.ChangePassword(r.Context(), principal.Subject, in.CurrentPassword, in.NewPassword); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeTokens(w http.ResponseWriter, p *usecase.TokenPair) {
	web.JSON(w, http.StatusOK, map[string]any{
		"access_token":  p.AccessToken,
		"refresh_token": p.RefreshToken,
		"token_type":    "Bearer",
		"expires_in":    p.ExpiresIn,
	})
}

func adminQInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// ListUsers returns a searchable, paginated user list with roles + subscriptions.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	role := r.URL.Query().Get("role")
	subscription := r.URL.Query().Get("subscription")
	sort := r.URL.Query().Get("sort")
	limit := adminQInt(r, "limit", 20)
	offset := adminQInt(r, "offset", 0)
	users, total, err := h.svc.ListUsers(r.Context(), q, role, subscription, sort, limit, offset)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"users": users, "total": total, "limit": limit, "offset": offset})
}

// SetUserRole toggles a role on a user (admin only). An admin cannot remove
// their own admin role, to avoid locking themselves out.
func (h *Handler) SetUserRole(w http.ResponseWriter, r *http.Request) {
	pr, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	userID := r.PathValue("id")
	var in struct {
		Role    string `json:"role"`
		Enabled bool   `json:"enabled"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if userID == pr.Subject && in.Role == "admin" && !in.Enabled {
		web.Error(w, r, apperr.New("auth.cannot_demote_self", apperr.KindValidation))
		return
	}
	if err := h.svc.SetUserRole(r.Context(), userID, in.Role, in.Enabled); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// UserDetail returns one user's full admin view (admin only).
func (h *Handler) UserDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.svc.UserDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, detail)
}

// BulkSetUserRole applies a role change to multiple users (admin only).
func (h *Handler) BulkSetUserRole(w http.ResponseWriter, r *http.Request) {
	pr, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		UserIDs []string `json:"user_ids"`
		Role    string   `json:"role"`
		Enabled bool     `json:"enabled"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.UserIDs) == 0 {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	updated, skipped, err := h.svc.BulkSetUserRole(r.Context(), in.UserIDs, in.Role, in.Enabled, pr.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"updated": updated, "skipped": skipped})
}

// SetUserStatus suspends/reactivates a user (admin only). An admin cannot
// suspend their own account.
func (h *Handler) SetUserStatus(w http.ResponseWriter, r *http.Request) {
	pr, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	userID := r.PathValue("id")
	var in struct {
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if userID == pr.Subject && in.Status == "suspended" {
		web.Error(w, r, apperr.New("auth.cannot_suspend_self", apperr.KindValidation))
		return
	}
	if err := h.svc.SetUserStatus(r.Context(), userID, in.Status); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
