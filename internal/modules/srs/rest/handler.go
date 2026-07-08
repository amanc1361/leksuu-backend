package rest

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/modules/srs/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func uid(r *http.Request) (string, bool) {
	p, ok := authn.FromContext(r.Context())
	return p.Subject, ok
}

func cardView(c *domain.Card) map[string]any {
	return map[string]any{"id": c.ID, "box": c.Box, "interval_days": c.IntervalDays, "ease": c.Ease, "reps": c.Reps, "lapses": c.Lapses, "suspended": c.Suspended, "due_at": c.DueAt}
}

// AddCard starts reviewing a word/phrase (idempotent per user+lemma).
func (h *Handler) AddCard(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Language string            `json:"language"`
		Kind     string            `json:"kind"`
		Lemma    string            `json:"lemma"`
		Example  string            `json:"example"`
		Source   string            `json:"source"`
		Meaning  map[string]string `json:"meaning"`
		Clip     *domain.Clip      `json:"clip"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	c, err := h.svc.AddCard(r.Context(), u, usecase.AddInput{
		Language: in.Language, Kind: in.Kind, Lemma: in.Lemma,
		Example: in.Example, Source: in.Source, Meaning: in.Meaning, Clip: in.Clip,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, cardView(c))
}

// Due returns cards ready to review now.
func (h *Handler) Due(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cards, err := h.svc.Due(r.Context(), u, r.URL.Query().Get("lang"), limit)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	if cards == nil {
		cards = []domain.DueCard{}
	}
	web.JSON(w, http.StatusOK, map[string]any{"cards": cards})
}

// Review records a grade and reschedules the card.
func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Grade string `json:"grade"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	c, err := h.svc.Review(r.Context(), u, r.PathValue("id"), domain.Grade(in.Grade))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, cardView(c))
}

// Relearn resumes a suspended/stubborn card and resets it to a fresh start.
func (h *Handler) Relearn(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	c, err := h.svc.Relearn(r.Context(), u, r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, cardView(c))
}

// Activity returns today's review count, the daily goal, and the review streak.
func (h *Handler) Activity(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	a, err := h.svc.Activity(r.Context(), u)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, a)
}

// WeekActivity returns the last 7 days of review counts for the dashboard strip.
func (h *Handler) WeekActivity(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	days, err := h.svc.WeekActivity(r.Context(), u)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"days": days})
}

// Settings returns the user's SRS preferences (the daily review goal).
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	st, err := h.svc.Settings(r.Context(), u)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, st)
}

// SetGoal updates the user's daily review goal.
func (h *Handler) SetGoal(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Goal int `json:"goal"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	st, err := h.svc.SetGoal(r.Context(), u, in.Goal)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, st)
}

// Stats is the review overview (due count + box distribution).
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	st, err := h.svc.Stats(r.Context(), u)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, st)
}

// Forecast returns the upcoming per-day review load for the maturity dashboard.
func (h *Handler) Forecast(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	days := 14
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d > 0 {
		days = d
	}
	fc, err := h.svc.Forecast(r.Context(), u, days)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	if fc == nil {
		fc = []domain.DayForecast{}
	}
	web.JSON(w, http.StatusOK, map[string]any{"days": fc})
}

// Leeches lists the learner's most stubborn cards for the dashboard.
func (h *Handler) Leeches(w http.ResponseWriter, r *http.Request) {
	u, ok := uid(r)
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}
	cards, err := h.svc.Leeches(r.Context(), u, limit)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	if cards == nil {
		cards = []domain.LeechCard{}
	}
	web.JSON(w, http.StatusOK, map[string]any{"cards": cards, "threshold": domain.LeechLapses})
}

func (h *Handler) Lookup(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.Lookup(r.Context(), r.URL.Query().Get("lang"), r.URL.Query().Get("lemma"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{
		"lemma": v.Lemma, "meaning": v.Meaning, "example": v.Example, "pronunciation": v.Pronunciation,
	})
}

// ListVocab returns vocabulary entries for browsing / in-class push.
func (h *Handler) ListVocab(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	language := q.Get("language")
	if language == "" {
		language = "en"
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	vs, total, err := h.svc.ListVocab(r.Context(), language, q.Get("q"), limit, offset)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(vs))
	for _, v := range vs {
		out = append(out, map[string]any{
			"id": v.ID, "language": v.Language, "kind": v.Kind, "lemma": v.Lemma,
			"pronunciation": v.Pronunciation, "meaning": v.Meaning, "example": v.Example,
		})
	}
	web.JSON(w, http.StatusOK, map[string]any{"vocab": out, "total": total})
}

// UndoReview restores the last review's schedule (one-level undo).
func (h *Handler) UndoReview(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	c, err := h.svc.Undo(r.Context(), p.Subject, r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"card_id": c.ID, "box": c.Box, "due_at": c.DueAt})
}

// MyCards lists the caller's cards for the word manager.
func (h *Handler) MyCards(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	cards, err := h.svc.MyCards(r.Context(), p.Subject, q.Get("query"), q.Get("status"), limit, offset)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"cards": cards})
}

// SuspendCard pauses/resumes a card: {"suspended": true|false}.
func (h *Handler) SuspendCard(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Suspended bool `json:"suspended"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	c, err := h.svc.Suspend(r.Context(), p.Subject, r.PathValue("id"), in.Suspended)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"card_id": c.ID, "suspended": c.Suspended})
}

// DeleteCard removes a card from the caller's deck.
func (h *Handler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if err := h.svc.DeleteCard(r.Context(), p.Subject, r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"deleted": true})
}
