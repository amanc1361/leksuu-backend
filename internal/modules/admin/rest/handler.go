// Package rest is the admin module's HTTP delivery layer. Every route is guarded
// by the admin role (RequireRole). It authors content the course module serves.
package rest

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/leksuu/platform/internal/modules/admin/usecase"
	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	srsdomain "github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func courseView(c coursedomain.Course) map[string]any {
	return map[string]any{
		"id": c.ID, "language": c.Language, "cefr": c.CEFR, "title": c.Title,
		"description": c.Description, "image_url": c.ImageURL, "version": c.Version, "published": c.Published,
	}
}

func qInt(r *http.Request, key string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return v
	}
	return def
}

func (h *Handler) ListCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := qInt(r, "limit", 20)
	offset := qInt(r, "offset", 0)
	courses, total, err := h.svc.SearchCourses(r.Context(), q, limit, offset)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(courses))
	for _, c := range courses {
		out = append(out, courseView(c))
	}
	web.JSON(w, http.StatusOK, map[string]any{"courses": out, "total": total})
}

func unitView(u coursedomain.Unit) map[string]any {
	return map[string]any{"id": u.ID, "course_id": u.CourseID, "position": u.Position, "title": u.Title, "description": u.Description}
}

func (h *Handler) ListUnits(w http.ResponseWriter, r *http.Request) {
	units, err := h.svc.Units(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(units))
	for _, u := range units {
		out = append(out, unitView(u))
	}
	web.JSON(w, http.StatusOK, map[string]any{"units": out})
}

func (h *Handler) CreateUnit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	u, err := h.svc.CreateUnit(r.Context(), r.PathValue("id"), in.Title, in.Description)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, unitView(*u))
}

func (h *Handler) ReorderUnits(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Order []string `json:"order"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Order) == 0 {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.ReorderUnits(r.Context(), r.PathValue("id"), in.Order); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListUnitLessons(w http.ResponseWriter, r *http.Request) {
	lessons, err := h.svc.LessonsByUnit(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(lessons))
	for _, l := range lessons {
		out = append(out, map[string]any{"id": l.ID, "position": l.Position, "title": l.Title, "unit_id": l.UnitID})
	}
	web.JSON(w, http.StatusOK, map[string]any{"lessons": out})
}

func (h *Handler) CreateUnitLesson(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title    string `json:"title"`
		Position int    `json:"position"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	l, err := h.svc.AddLessonToUnit(r.Context(), r.PathValue("id"), in.Title, in.Position)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{"id": l.ID, "position": l.Position, "title": l.Title, "unit_id": l.UnitID})
}

func (h *Handler) ReorderLessons(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Order []string `json:"order"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Order) == 0 {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.ReorderLessons(r.Context(), r.PathValue("id"), in.Order); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateCourse(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Language    string
		CEFR        string
		Title       string
		Description string
		ImageURL    string `json:"image_url"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	c, err := h.svc.CreateCourse(r.Context(), usecase.CreateCourseInput(in))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, courseView(*c))
}

func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Published bool `json:"published"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.Publish(r.Context(), r.PathValue("id"), in.Published); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "published": in.Published})
}

func (h *Handler) ListLessons(w http.ResponseWriter, r *http.Request) {
	lessons, err := h.svc.Lessons(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(lessons))
	for _, l := range lessons {
		out = append(out, map[string]any{"id": l.ID, "position": l.Position, "title": l.Title})
	}
	web.JSON(w, http.StatusOK, map[string]any{"lessons": out})
}

func (h *Handler) CreateLesson(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title    string `json:"title"`
		Position int    `json:"position"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	l, err := h.svc.AddLesson(r.Context(), r.PathValue("id"), in.Title, in.Position)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{"id": l.ID, "position": l.Position, "title": l.Title})
}

func lessonAdminView(l coursedomain.Lesson) map[string]any {
	obj := l.Objective
	if len(obj) == 0 {
		obj = json.RawMessage("{}")
	}
	return map[string]any{
		"id": l.ID, "title": l.Title, "position": l.Position, "unit_id": l.UnitID,
		"cover_url": l.CoverURL, "focus": l.Focus, "objective": obj,
	}
}

func (h *Handler) UpdateLesson(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title     string          `json:"title"`
		Focus     string          `json:"focus"`
		CoverURL  string          `json:"cover_url"`
		Objective json.RawMessage `json:"objective"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	l, err := h.svc.UpdateLesson(r.Context(), r.PathValue("id"), in.Title, in.Focus, in.CoverURL, in.Objective)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, lessonAdminView(*l))
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind     string          `json:"kind"`
		Position int             `json:"position"`
		Payload  json.RawMessage `json:"payload"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	it, err := h.svc.AddItem(r.Context(), r.PathValue("id"), in.Kind, in.Payload, in.Position)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{"id": it.ID, "position": it.Position, "kind": it.Kind})
}

func (h *Handler) CreateExercise(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Type       string          `json:"type"`
		Prompt     json.RawMessage `json:"prompt"`
		AnswerKey  json.RawMessage `json:"answer_key"`
		Difficulty int             `json:"difficulty"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	e, err := h.svc.AddExercise(r.Context(), r.PathValue("id"), in.Type, in.Prompt, in.AnswerKey, in.Difficulty)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{"id": e.ID, "type": e.Type})
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Items(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{"id": it.ID, "position": it.Position, "kind": it.Kind, "payload": it.Payload, "media_key": it.MediaKey})
	}
	web.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Payload  json.RawMessage `json:"payload"`
		MediaKey *string         `json:"media_key"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	it, err := h.svc.UpdateItem(r.Context(), r.PathValue("id"), in.Payload, in.MediaKey)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"id": it.ID, "kind": it.Kind, "position": it.Position})
}

func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteItem(r.Context(), r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ReorderItems(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Order []string `json:"order"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Order) == 0 {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.ReorderItems(r.Context(), r.PathValue("id"), in.Order); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	web.JSON(w, http.StatusOK, map[string]any{"templates": h.svc.LessonTemplates()})
}

func (h *Handler) LessonContext(w http.ResponseWriter, r *http.Request) {
	cx, err := h.svc.LessonContextOf(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := map[string]any{
		"lesson": lessonAdminView(cx.Lesson),
	}
	if cx.Unit != nil {
		out["unit"] = unitView(*cx.Unit)
	}
	if cx.Course != nil {
		out["course"] = map[string]any{"id": cx.Course.ID, "cefr": cx.Course.CEFR, "language": cx.Course.Language, "title": cx.Course.Title}
	}
	web.JSON(w, http.StatusOK, out)
}

func vocabView(v srsdomain.Vocab) map[string]any {
	return map[string]any{
		"id": v.ID, "language": v.Language, "kind": v.Kind, "lemma": v.Lemma,
		"meaning": v.Meaning, "example": v.Example, "pronunciation": v.Pronunciation,
	}
}

func (h *Handler) ListVocab(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.svc.SearchVocab(r.Context(), r.URL.Query().Get("lang"), r.URL.Query().Get("q"), qInt(r, "limit", 30), qInt(r, "offset", 0))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, v := range items {
		out = append(out, vocabView(v))
	}
	web.JSON(w, http.StatusOK, map[string]any{"items": out, "total": total})
}

func (h *Handler) CreateVocab(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Language      string            `json:"language"`
		Kind          string            `json:"kind"`
		Lemma         string            `json:"lemma"`
		Meaning       map[string]string `json:"meaning"`
		Example       string            `json:"example"`
		Pronunciation string            `json:"pronunciation"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	v, err := h.svc.CreateVocab(r.Context(), in.Language, in.Kind, in.Lemma, in.Meaning, in.Example, in.Pronunciation)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, vocabView(*v))
}

func (h *Handler) UpdateVocab(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Meaning       map[string]string `json:"meaning"`
		Example       string            `json:"example"`
		Pronunciation string            `json:"pronunciation"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	v, err := h.svc.UpdateVocab(r.Context(), r.PathValue("id"), in.Meaning, in.Example, in.Pronunciation)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, vocabView(*v))
}

func (h *Handler) DeleteVocab(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteVocab(r.Context(), r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetAccess pins or clears the access requirement of a course/unit/lesson.
func (h *Handler) SetAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		EntityType  string `json:"entity_type"`
		EntityID    string `json:"entity_id"`
		Requirement string `json:"requirement"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.SetAccess(r.Context(), in.EntityType, in.EntityID, in.Requirement); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// AccessRules lists all explicit access rules.
func (h *Handler) AccessRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.AccessRules(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"rules": rules})
}

// ImportMedia publishes a standalone film/podcast into the learner media library
// in one request — no course/unit/lesson navigation. POST /admin/media.
func (h *Handler) ImportMedia(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind       string             `json:"kind"`
		Title      string             `json:"title"`
		Language   string             `json:"language"`
		CEFR       string             `json:"cefr"`
		URL        string             `json:"url"`
		Cover      string             `json:"cover"`
		Transcript []usecase.MediaCue `json:"transcript"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	it, err := h.svc.ImportMedia(r.Context(), usecase.ImportMediaInput{
		Kind: in.Kind, Title: in.Title, Language: in.Language, CEFR: in.CEFR, URL: in.URL, Cover: in.Cover, Transcript: in.Transcript,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{"id": it.ID, "kind": it.Kind, "lesson_id": it.LessonID})
}

// UpdateMedia rewrites a media item's payload from the standalone editor.
// PATCH /admin/media/{id}.
func (h *Handler) UpdateMedia(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title      string             `json:"title"`
		Language   string             `json:"language"`
		CEFR       string             `json:"cefr"`
		URL        string             `json:"url"`
		Cover      string             `json:"cover"`
		Transcript []usecase.MediaCue `json:"transcript"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	it, err := h.svc.UpdateMedia(r.Context(), r.PathValue("id"), usecase.ImportMediaInput{
		Title: in.Title, Language: in.Language, CEFR: in.CEFR, URL: in.URL, Cover: in.Cover, Transcript: in.Transcript,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"id": it.ID})
}
