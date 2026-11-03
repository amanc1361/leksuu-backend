// Package rest is the course module's HTTP delivery layer. Browse endpoints are
// public; progress endpoints require authentication (via an injected Guard that
// publishes the shared principal). Exercise answer keys are NEVER serialized.
package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/modules/course/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct {
	svc            *usecase.Service
	premium        Premium // optional; gates film/podcast (media library)
	targetLanguage func(context.Context, string) (string, error)
}

func NewHandler(svc *usecase.Service, premium Premium, targetLanguage ...func(context.Context, string) (string, error)) *Handler {
	h := &Handler{svc: svc, premium: premium}
	if len(targetLanguage) > 0 {
		h.targetLanguage = targetLanguage[0]
	}
	return h
}

// --- presenters (decide exactly what leaves the server) ---

func courseView(c domain.Course) map[string]any {
	return map[string]any{
		"id": c.ID, "language": c.Language, "cefr": c.CEFR,
		"title": c.Title, "description": c.Description, "image_url": c.ImageURL, "version": c.Version,
	}
}

func lessonView(l domain.Lesson) map[string]any {
	obj := l.Objective
	if len(obj) == 0 {
		obj = json.RawMessage("{}")
	}
	return map[string]any{
		"id": l.ID, "position": l.Position, "title": l.Title, "unit_id": l.UnitID,
		"cover_url": l.CoverURL, "focus": l.Focus, "kind": lessonKind(l.Kind), "objective": obj,
	}
}

// exerciseView omits answer_key on purpose. For single-word fill answers it adds
// `answer_len` (the letter count only — never the word) so the client can render
// exactly that many input cells.
func exerciseView(e domain.Exercise) map[string]any {
	var prompt map[string]any
	if len(e.Prompt) > 0 {
		_ = json.Unmarshal(e.Prompt, &prompt)
	}
	if prompt == nil {
		prompt = map[string]any{}
	}
	if e.Type == "fill_blank" || e.Type == "dictation" {
		var key struct {
			Answers []string `json:"answers"`
		}
		if json.Unmarshal(e.AnswerKey, &key) == nil && len(key.Answers) > 0 {
			a := strings.TrimSpace(key.Answers[0])
			if a != "" && !strings.ContainsAny(a, " \t\n") {
				// Letter cells only accept letters: an answer with an apostrophe
				// or hyphen ("aren't", "twenty-one") gets the free-text input.
				if n := utf8.RuneCountInString(a); n >= 1 && n <= 24 && onlyLetters(a) {
					prompt["answer_len"] = n
				}
				// Scaffold: reveal only the first letter, never the word.
				if r := []rune(a); len(r) > 0 {
					prompt["assist"] = map[string]any{"kind": "first_letter", "value": string(r[0])}
				}
			}
		}
	}
	// A safe hint per type: partial help that never exposes the full solution.
	switch e.Type {
	case "mcq", "listen_choose":
		var key struct {
			Correct int `json:"correct"`
		}
		if json.Unmarshal(e.AnswerKey, &key) == nil {
			n := 0
			if opts, ok := prompt["options"].([]any); ok {
				n = len(opts)
			}
			if n >= 3 { // only worth eliminating when 3+ options remain
				wrong := 0
				if key.Correct == 0 {
					wrong = 1
				}
				if wrong < n && wrong != key.Correct {
					prompt["assist"] = map[string]any{"kind": "eliminate", "index": wrong}
				}
			}
		}
	case "reorder", "sentence_builder":
		var key struct {
			Order []string `json:"order"`
		}
		if json.Unmarshal(e.AnswerKey, &key) == nil && len(key.Order) > 0 {
			prompt["assist"] = map[string]any{"kind": "first_word", "value": key.Order[0]}
		}
	case "select_words":
		// How many tiles to pick ("Choose the two negative forms") — the count,
		// never which ones.
		var key struct {
			Selected []int `json:"selected"`
		}
		if json.Unmarshal(e.AnswerKey, &key) == nil && len(key.Selected) > 0 {
			prompt["select_count"] = len(key.Selected)
		}
	case "fill_gaps":
		// How many gaps to fill — never their answers.
		var key struct {
			Gaps [][]string `json:"gaps"`
		}
		if json.Unmarshal(e.AnswerKey, &key) == nil && len(key.Gaps) > 0 {
			prompt["gap_count"] = len(key.Gaps)
		}
	case "match", "match_image":
		var key struct {
			Pairs [][]string `json:"pairs"`
		}
		if json.Unmarshal(e.AnswerKey, &key) == nil && len(key.Pairs) > 0 && len(key.Pairs[0]) == 2 {
			prompt["assist"] = map[string]any{"kind": "pair", "left": key.Pairs[0][0], "right": key.Pairs[0][1]}
		}
	}
	return map[string]any{"id": e.ID, "type": e.Type, "prompt": prompt, "difficulty": e.Difficulty}
}

func onlyLetters(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func itemView(iwe usecase.ItemWithExercises) map[string]any {
	exs := make([]map[string]any, 0, len(iwe.Exercises))
	for _, e := range iwe.Exercises {
		exs = append(exs, exerciseView(e))
	}
	return map[string]any{
		"id": iwe.Item.ID, "position": iwe.Item.Position, "kind": iwe.Item.Kind,
		"payload": iwe.Item.Payload, "media_key": iwe.Item.MediaKey, "ref_id": iwe.Item.RefID,
		"exercises": exs,
	}
}

// --- public browse ---

func (h *Handler) ListCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	courses, err := h.svc.ListCourses(r.Context(), q.Get("language"), q.Get("cefr"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(courses))
	for _, c := range courses {
		out = append(out, courseView(c))
	}
	web.JSON(w, http.StatusOK, map[string]any{"courses": out})
}

// viewerOf is the caller of a request: anonymous unless a principal was
// published by the auth middleware.
func viewerOf(r *http.Request) usecase.Viewer {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		return usecase.Viewer{}
	}
	return usecase.Viewer{UserID: p.Subject, Admin: p.HasRole("admin"), Account: p.AccountType()}
}

func (h *Handler) GetCourse(w http.ResponseWriter, r *http.Request) {
	cwl, err := h.svc.GetCourse(r.Context(), viewerOf(r), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	// units carries the chapter hierarchy; lessons is the same lessons in
	// global learning order (unit by unit). "position" is per unit, so clients
	// must use this order (or units), never sort lessons by position.
	units := make([]map[string]any, 0, len(cwl.Units))
	for _, u := range cwl.Units {
		ls := make([]map[string]any, 0, len(u.Lessons))
		for _, l := range u.Lessons {
			ls = append(ls, lessonView(l))
		}
		units = append(units, map[string]any{
			"id": u.Unit.ID, "position": u.Unit.Position, "title": u.Unit.Title,
			"description": u.Unit.Description, "lessons": ls,
		})
	}
	lessons := make([]map[string]any, 0, len(cwl.Lessons))
	for i, l := range cwl.Lessons {
		v := lessonView(l)
		v["sequence"] = i + 1
		lessons = append(lessons, v)
	}
	body := courseView(cwl.Course)
	body["units"] = units
	body["lessons"] = lessons
	web.JSON(w, http.StatusOK, body)
}

func mediaQInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (h *Handler) MediaLibrary(w http.ResponseWriter, r *http.Request) {
	if !learnerRequest(r) {
		web.Error(w, r, domain.ErrLearnerOnly)
		return
	}
	if h.premium != nil {
		pr, ok := authn.FromContext(r.Context())
		if !ok {
			web.Error(w, r, apperr.ErrInternal)
			return
		}
		if !pr.HasRole("admin") {
			if err := h.premium.RequirePremium(r.Context(), pr.Subject); err != nil {
				web.Error(w, r, err)
				return
			}
		}
	}
	language := r.URL.Query().Get("lang")
	if h.targetLanguage != nil {
		principal, ok := authn.FromContext(r.Context())
		if !ok {
			web.Error(w, r, apperr.ErrInternal)
			return
		}
		var err error
		language, err = h.targetLanguage(r.Context(), principal.Subject)
		if err != nil {
			web.Error(w, r, err)
			return
		}
	}
	if language == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	kind := r.URL.Query().Get("type")
	if kind != domain.KindStory && kind != domain.KindPodcast && kind != domain.KindVideo {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	items, total, err := h.svc.MediaLibrary(r.Context(), kind, language, mediaQInt(r, "limit", 50), mediaQInt(r, "offset", 0))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{"id": it.ID, "kind": it.Kind, "lesson_id": it.LessonID, "payload": it.Payload, "media_key": it.MediaKey})
	}
	web.JSON(w, http.StatusOK, map[string]any{"items": out, "total": total, "language": language})
}

func (h *Handler) GetLesson(w http.ResponseWriter, r *http.Request) {
	lwi, err := h.svc.GetLesson(r.Context(), viewerOf(r), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(lwi.Items))
	for _, it := range lwi.Items {
		items = append(items, itemView(it))
	}
	body := lessonView(lwi.Lesson)
	body["course_id"] = lwi.Lesson.CourseID
	body["language"] = lwi.Language
	body["items"] = items
	body["locked"] = lwi.Locked
	web.JSON(w, http.StatusOK, body)
}

// --- protected progress ---

func (h *Handler) CompleteLesson(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	res, err := h.svc.CompleteLesson(r.Context(), usecase.Viewer{UserID: p.Subject, Admin: p.HasRole("admin")}, r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := map[string]any{"status": domain.StatusCompleted, "awarded_xp": res.AwardedXP, "passed": res.Passed}
	if res.Preview {
		out["preview"] = true
	}
	if res.Checkpoint {
		out["checkpoint"] = true
		out["score"] = res.Score
		out["pass_score"] = usecase.CheckpointPassRatio
		if !res.Passed {
			out["status"] = "not_passed"
		}
	}
	web.JSON(w, http.StatusOK, out)
}

func (h *Handler) AnswerExercise(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Answer json.RawMessage `json:"answer"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	res, err := h.svc.GradeExercise(r.Context(), usecase.Viewer{UserID: p.Subject, Admin: p.HasRole("admin")}, r.PathValue("id"), in.Answer)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := map[string]any{"correct": res.Correct, "score": res.Score, "solution": res.Solution, "first_try": res.FirstTry, "awarded_xp": res.AwardedXP}
	if res.Preview {
		out["preview"] = true
	}
	web.JSON(w, http.StatusOK, out)
}

func (h *Handler) CourseProgress(w http.ResponseWriter, r *http.Request) {
	if !learnerRequest(r) {
		web.Error(w, r, domain.ErrLearnerOnly)
		return
	}
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	sum, err := h.svc.CourseProgress(r.Context(), p.Subject, r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{
		"total_lessons": sum.TotalLessons, "completed_lessons": sum.CompletedLessons, "percent": sum.Percent,
		"completed_lesson_ids": sum.CompletedIDs,
	})
}

// QuickPractice serves a small set of random gradeable exercises for a light
// review session. Answer keys are stripped via exerciseView, same as browse.
func (h *Handler) QuickPractice(w http.ResponseWriter, r *http.Request) {
	if !learnerRequest(r) {
		web.Error(w, r, domain.ErrLearnerOnly)
		return
	}
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	n := mediaQInt(r, "n", 5)
	exs, err := h.svc.QuickPractice(r.Context(), p.Subject, n)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(exs))
	for _, e := range exs {
		out = append(out, exerciseView(e))
	}
	web.JSON(w, http.StatusOK, map[string]any{"exercises": out})
}

// lessonKind exposes a lesson's kind ("lesson" or "checkpoint").
func lessonKind(k string) string { return domain.KindOrDefault(k) }

// ListLanguages returns the target languages learners can choose, in order.
func (h *Handler) ListLanguages(w http.ResponseWriter, r *http.Request) {
	langs, err := h.svc.Languages(r.Context(), false)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(langs))
	for _, l := range langs {
		out = append(out, map[string]any{"code": l.Code, "position": l.Position})
	}
	web.JSON(w, http.StatusOK, map[string]any{"languages": out})
}

// learnerRequest: true for anonymous callers and learner accounts. Teacher and
// admin accounts have their own workspaces and get no learner progress views.
func learnerRequest(r *http.Request) bool {
	p, ok := authn.FromContext(r.Context())
	return !ok || p.AccountType() == authn.AccountLearner
}
