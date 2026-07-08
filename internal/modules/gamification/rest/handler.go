// Package rest exposes the gamification snapshot. Read-only and authenticated;
// XP is awarded internally by other modules, never by direct client request.
package rest

import (
	"encoding/json"
	"net/http"

	"github.com/leksuu/platform/internal/modules/gamification/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func stateBody(view *usecase.StateView) map[string]any {
	return map[string]any{
		"xp":             view.State.XP,
		"gems":           view.State.Gems,
		"streak":         view.State.Streak,
		"longest_streak": view.State.LongestStreak,
		"league":         view.State.League,
		"daily_goal": map[string]any{
			"target": view.Today.TargetXP,
			"earned": view.Today.EarnedXP,
			"met":    view.Today.Met,
		},
	}
}

func (h *Handler) GetState(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	view, err := h.svc.GetState(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, stateBody(view))
}

type Guard interface {
	Require(next http.Handler) http.Handler
	RequireRole(role string, next http.Handler) http.Handler
}

func (h *Handler) SetDailyGoal(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		TargetXP int `json:"target_xp"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	view, err := h.svc.SetDailyTarget(r.Context(), p.Subject, in.TargetXP)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, stateBody(view))
}

func rewardBody(v *usecase.RewardView) map[string]any {
	return map[string]any{"available": v.Available, "amount": v.Amount, "gems": v.Gems, "streak": v.Streak}
}

func (h *Handler) DailyReward(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	v, err := h.svc.DailyRewardStatus(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, rewardBody(v))
}

func (h *Handler) ClaimDailyReward(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	v, err := h.svc.ClaimDailyReward(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, rewardBody(v))
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("GET "+prefix+"/gamification", guard.Require(http.HandlerFunc(h.GetState)))
	mux.Handle("PUT "+prefix+"/gamification/daily-goal", guard.Require(http.HandlerFunc(h.SetDailyGoal)))
	mux.Handle("GET "+prefix+"/gamification/daily-reward", guard.Require(http.HandlerFunc(h.DailyReward)))
	mux.Handle("POST "+prefix+"/gamification/daily-reward/claim", guard.Require(http.HandlerFunc(h.ClaimDailyReward)))
	mux.Handle("GET "+prefix+"/gamification/daily-mix", guard.Require(http.HandlerFunc(h.DailyMix)))
	mux.Handle("POST "+prefix+"/gamification/daily-mix/complete", guard.Require(http.HandlerFunc(h.CompleteDailyMix)))
	mux.Handle("GET "+prefix+"/gamification/leaderboard", guard.Require(http.HandlerFunc(h.Leaderboard)))
	mux.Handle("POST "+prefix+"/admin/recaps/run", guard.RequireRole("admin", http.HandlerFunc(h.RunRecaps)))
	mux.Handle("GET "+prefix+"/admin/users/{id}/activity", guard.RequireRole("admin", http.HandlerFunc(h.UserActivity)))
}

// UserActivity returns a user's learning-activity snapshot (admin only).
func (h *Handler) UserActivity(w http.ResponseWriter, r *http.Request) {
	act, err := h.svc.UserActivity(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, act)
}

// DailyMix reports today's guided-session bonus status.
func (h *Handler) DailyMix(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	v, err := h.svc.DailyMixStatus(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, v)
}

// CompleteDailyMix awards the once-per-day mix bonus (idempotent).
func (h *Handler) CompleteDailyMix(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	v, err := h.svc.CompleteDailyMix(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, v)
}

// Leaderboard returns this week's XP league standing.
func (h *Handler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	v, err := h.svc.Leaderboard(r.Context(), p.Subject, 20)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, v)
}

// RunRecaps triggers the weekly recap job immediately (admin/ops).
func (h *Handler) RunRecaps(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.SendWeeklyRecaps(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"sent": n})
}
