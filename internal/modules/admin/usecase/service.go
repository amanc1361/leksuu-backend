// Package usecase holds content-authoring rules for the admin module. It reuses
// the course bounded context's entities and writes through a Repository the
// content store satisfies (the same store the course module reads from).
package usecase

import (
	"context"
	"encoding/json"
	"strings"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	srsdomain "github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Repository is the content write+read surface authoring needs (satisfied by the
// shared content store / Postgres repo).
type Repository interface {
	ListAllCourses(ctx context.Context) ([]coursedomain.Course, error)
	CreateCourse(ctx context.Context, c *coursedomain.Course) error
	Course(ctx context.Context, id string) (*coursedomain.Course, error)
	SetPublished(ctx context.Context, id string, published bool) error
	CreateLesson(ctx context.Context, l *coursedomain.Lesson) error
	UpdateLesson(ctx context.Context, l *coursedomain.Lesson) error
	Lesson(ctx context.Context, id string) (*coursedomain.Lesson, error)
	Lessons(ctx context.Context, courseID string) ([]coursedomain.Lesson, error)
	CreateItem(ctx context.Context, it *coursedomain.LessonItem) error
	UpdateItem(ctx context.Context, it *coursedomain.LessonItem) error
	CreateExercise(ctx context.Context, e *coursedomain.Exercise) error
	Items(ctx context.Context, lessonID string) ([]coursedomain.LessonItem, error)
	DeleteItem(ctx context.Context, itemID string) error
	ReorderItems(ctx context.Context, lessonID string, orderedIDs []string) error
	Units(ctx context.Context, courseID string) ([]coursedomain.Unit, error)
	Unit(ctx context.Context, id string) (*coursedomain.Unit, error)
	CreateUnit(ctx context.Context, u *coursedomain.Unit) error
	LessonsByUnit(ctx context.Context, unitID string) ([]coursedomain.Lesson, error)
	ReorderUnits(ctx context.Context, courseID string, orderedIDs []string) error
	ReorderLessons(ctx context.Context, unitID string, orderedIDs []string) error
	SearchCourses(ctx context.Context, q string, limit, offset int) ([]coursedomain.Course, int, error)
	SetAccessRule(ctx context.Context, rule coursedomain.AccessRule) error
	DeleteAccessRule(ctx context.Context, entityType, entityID string) error
	ListAccessRules(ctx context.Context) ([]coursedomain.AccessRule, error)
}

// VocabStore is the admin-facing slice of the vocabulary bank (vocabulary_items),
// satisfied structurally by the SRS repository.
type VocabStore interface {
	ListVocab(ctx context.Context, language, q string, limit, offset int) ([]srsdomain.Vocab, int, error)
	GetVocab(ctx context.Context, id string) (*srsdomain.Vocab, error)
	EnsureVocab(ctx context.Context, v *srsdomain.Vocab) error
	UpdateVocab(ctx context.Context, v *srsdomain.Vocab) error
	DeleteVocab(ctx context.Context, id string) error
}

type Service struct {
	repo  Repository
	vocab VocabStore
}

func NewService(repo Repository, vocab VocabStore) *Service {
	return &Service{repo: repo, vocab: vocab}
}

func (s *Service) ListCourses(ctx context.Context) ([]coursedomain.Course, error) {
	return s.repo.ListAllCourses(ctx)
}

type CreateCourseInput struct{ Language, CEFR, Title, Description, ImageURL string }

// CreateCourse authors a new course in any target language (unpublished).
func (s *Service) CreateCourse(ctx context.Context, in CreateCourseInput) (*coursedomain.Course, error) {
	if in.Language == "" || in.Title == "" {
		return nil, apperr.ErrBadRequest
	}
	cefr := in.CEFR
	if cefr == "" {
		cefr = "A1"
	}
	c := &coursedomain.Course{Language: in.Language, CEFR: cefr, Title: in.Title, Description: in.Description, ImageURL: in.ImageURL, Published: false}
	if err := s.repo.CreateCourse(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) Publish(ctx context.Context, courseID string, published bool) error {
	if _, err := s.repo.Course(ctx, courseID); err != nil {
		return err
	}
	return s.repo.SetPublished(ctx, courseID, published)
}

func (s *Service) Lessons(ctx context.Context, courseID string) ([]coursedomain.Lesson, error) {
	return s.repo.Lessons(ctx, courseID)
}

func (s *Service) Items(ctx context.Context, lessonID string) ([]coursedomain.LessonItem, error) {
	return s.repo.Items(ctx, lessonID)
}

func (s *Service) DeleteItem(ctx context.Context, itemID string) error {
	return s.repo.DeleteItem(ctx, itemID)
}

func (s *Service) ReorderItems(ctx context.Context, lessonID string, orderedIDs []string) error {
	return s.repo.ReorderItems(ctx, lessonID, orderedIDs)
}

func (s *Service) SearchCourses(ctx context.Context, q string, limit, offset int) ([]coursedomain.Course, int, error) {
	return s.repo.SearchCourses(ctx, q, limit, offset)
}

func (s *Service) Units(ctx context.Context, courseID string) ([]coursedomain.Unit, error) {
	return s.repo.Units(ctx, courseID)
}

func (s *Service) CreateUnit(ctx context.Context, courseID, title, description string) (*coursedomain.Unit, error) {
	if title == "" {
		return nil, apperr.ErrBadRequest
	}
	if _, err := s.repo.Course(ctx, courseID); err != nil {
		return nil, err
	}
	u := &coursedomain.Unit{CourseID: courseID, Title: title, Description: description}
	if err := s.repo.CreateUnit(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) ReorderUnits(ctx context.Context, courseID string, orderedIDs []string) error {
	return s.repo.ReorderUnits(ctx, courseID, orderedIDs)
}

func (s *Service) LessonsByUnit(ctx context.Context, unitID string) ([]coursedomain.Lesson, error) {
	return s.repo.LessonsByUnit(ctx, unitID)
}

func (s *Service) ReorderLessons(ctx context.Context, unitID string, orderedIDs []string) error {
	return s.repo.ReorderLessons(ctx, unitID, orderedIDs)
}

func (s *Service) AddLessonToUnit(ctx context.Context, unitID, title string, position int) (*coursedomain.Lesson, error) {
	if title == "" {
		return nil, apperr.ErrBadRequest
	}
	if _, err := s.repo.Unit(ctx, unitID); err != nil {
		return nil, err
	}
	l := &coursedomain.Lesson{UnitID: unitID, Title: title, Position: position}
	if err := s.repo.CreateLesson(ctx, l); err != nil {
		return nil, err
	}
	return l, nil
}

func (s *Service) UpdateLesson(ctx context.Context, id, title, focus, coverURL string, objective json.RawMessage) (*coursedomain.Lesson, error) {
	l, err := s.repo.Lesson(ctx, id)
	if err != nil {
		return nil, err
	}
	if title != "" {
		l.Title = title
	}
	l.Focus = focus
	l.CoverURL = coverURL
	if len(objective) > 0 {
		l.Objective = objective
	}
	if err := s.repo.UpdateLesson(ctx, l); err != nil {
		return nil, err
	}
	return l, nil
}

func (s *Service) AddLesson(ctx context.Context, courseID, title string, position int) (*coursedomain.Lesson, error) {
	if title == "" {
		return nil, apperr.ErrBadRequest
	}
	if _, err := s.repo.Course(ctx, courseID); err != nil {
		return nil, err
	}
	l := &coursedomain.Lesson{CourseID: courseID, Title: title, Position: position}
	if err := s.repo.CreateLesson(ctx, l); err != nil {
		return nil, err
	}
	return l, nil
}

func (s *Service) AddItem(ctx context.Context, lessonID, kind string, payload json.RawMessage, position int) (*coursedomain.LessonItem, error) {
	if kind == "" {
		return nil, apperr.ErrBadRequest
	}
	it := &coursedomain.LessonItem{LessonID: lessonID, Kind: kind, Payload: payload, Position: position}
	if err := s.repo.CreateItem(ctx, it); err != nil {
		return nil, err
	}
	return it, nil
}

func (s *Service) UpdateItem(ctx context.Context, itemID string, payload json.RawMessage, mediaKey *string) (*coursedomain.LessonItem, error) {
	if itemID == "" {
		return nil, apperr.ErrBadRequest
	}
	it := &coursedomain.LessonItem{ID: itemID, Payload: payload, MediaKey: mediaKey}
	if err := s.repo.UpdateItem(ctx, it); err != nil {
		return nil, err
	}
	return it, nil
}

func (s *Service) AddExercise(ctx context.Context, itemID, typ string, prompt, answerKey json.RawMessage, difficulty int) (*coursedomain.Exercise, error) {
	if typ == "" {
		return nil, apperr.ErrBadRequest
	}
	e := &coursedomain.Exercise{LessonItemID: itemID, Type: typ, Prompt: prompt, AnswerKey: answerKey, Difficulty: difficulty}
	if err := s.repo.CreateExercise(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Service) LessonTemplates() []LessonTemplate { return lessonTemplates() }

// LessonContextResult is a lesson plus its unit and course summaries, used by the
// authoring UI for breadcrumbs and level-aware lesson templates.
type LessonContextResult struct {
	Lesson coursedomain.Lesson
	Unit   *coursedomain.Unit
	Course *coursedomain.Course
}

func (s *Service) LessonContextOf(ctx context.Context, lessonID string) (*LessonContextResult, error) {
	l, err := s.repo.Lesson(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	res := &LessonContextResult{Lesson: *l}
	if l.UnitID != "" {
		res.Unit, _ = s.repo.Unit(ctx, l.UnitID)
	}
	if l.CourseID != "" {
		res.Course, _ = s.repo.Course(ctx, l.CourseID)
	}
	return res, nil
}

func (s *Service) SearchVocab(ctx context.Context, language, q string, limit, offset int) ([]srsdomain.Vocab, int, error) {
	return s.vocab.ListVocab(ctx, language, q, limit, offset)
}

func (s *Service) CreateVocab(ctx context.Context, language, kind, lemma string, meaning map[string]string, example, pronunciation string) (*srsdomain.Vocab, error) {
	lemma = strings.ToLower(strings.TrimSpace(lemma))
	if language == "" || lemma == "" {
		return nil, apperr.ErrBadRequest
	}
	if kind == "" {
		kind = "word"
	}
	v := &srsdomain.Vocab{Language: language, Kind: kind, Lemma: lemma, Meaning: meaning, Example: example, Pronunciation: pronunciation}
	if err := s.vocab.EnsureVocab(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Service) UpdateVocab(ctx context.Context, id string, meaning map[string]string, example, pronunciation string) (*srsdomain.Vocab, error) {
	existing, err := s.vocab.GetVocab(ctx, id)
	if err != nil {
		return nil, err
	}
	existing.Meaning = meaning
	existing.Example = example
	existing.Pronunciation = pronunciation
	if err := s.vocab.UpdateVocab(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *Service) DeleteVocab(ctx context.Context, id string) error {
	return s.vocab.DeleteVocab(ctx, id)
}

// SetAccess pins or clears the access requirement of a course/unit/lesson.
func (s *Service) SetAccess(ctx context.Context, entityType, entityID, requirement string) error {
	switch entityType {
	case "course", "unit", "lesson":
	default:
		return apperr.ErrBadRequest
	}
	if requirement == "" || requirement == "inherit" {
		return s.repo.DeleteAccessRule(ctx, entityType, entityID)
	}
	if requirement != coursedomain.AccessFree && requirement != coursedomain.AccessCourses && requirement != coursedomain.AccessClasses {
		return apperr.ErrBadRequest
	}
	return s.repo.SetAccessRule(ctx, coursedomain.AccessRule{EntityType: entityType, EntityID: entityID, Requirement: requirement})
}

// AccessRules lists all explicit access rules for the admin console.
func (s *Service) AccessRules(ctx context.Context) ([]coursedomain.AccessRule, error) {
	return s.repo.ListAccessRules(ctx)
}

// MediaBucketTitle marks the hidden system course that hosts standalone media
// library items (films/podcasts) so admins never navigate the course tree to
// publish one. It stays unpublished, so it never appears in the learner catalogue.
const MediaBucketTitle = "__media_library__"

// MediaCue is one transcript line. Start/End are optional: untimed lines are
// valid for podcasts, and films can be timed later in the lesson editor.
// Translation maps a UI locale ("fa"/"ckb") to the line's translation.
type MediaCue struct {
	Start       *float64          `json:"start,omitempty"`
	End         *float64          `json:"end,omitempty"`
	Text        string            `json:"text"`
	Translation map[string]string `json:"translation,omitempty"`
}

// ImportMediaInput is a one-shot film/podcast publish.
type ImportMediaInput struct {
	Kind       string
	Title      string
	Language   string
	CEFR       string
	URL        string
	Cover      string
	Transcript []MediaCue
}

// ImportMedia publishes a standalone film/podcast straight into the learner
// media library, creating the hidden system bucket on first use. No course-tree
// navigation needed; the item reuses the LessonItem machinery (transcript,
// shadowing, clip-replay) so nothing downstream changes.
func (s *Service) ImportMedia(ctx context.Context, in ImportMediaInput) (*coursedomain.LessonItem, error) {
	kind := strings.TrimSpace(in.Kind)
	if kind != coursedomain.KindVideo && kind != coursedomain.KindPodcast && kind != coursedomain.KindStory {
		return nil, apperr.ErrBadRequest
	}
	// Stories are text-first graded readers: no media URL required.
	if strings.TrimSpace(in.Title) == "" || len(in.Transcript) == 0 ||
		(kind != coursedomain.KindStory && strings.TrimSpace(in.URL) == "") {
		return nil, apperr.ErrBadRequest
	}
	lessonID, err := s.ensureMediaLesson(ctx)
	if err != nil {
		return nil, err
	}
	lang := strings.TrimSpace(in.Language)
	if lang == "" {
		lang = "en"
	}
	payload, err := json.Marshal(map[string]any{
		"title":      strings.TrimSpace(in.Title),
		"language":   lang,
		"level":      strings.TrimSpace(in.CEFR),
		"url":        strings.TrimSpace(in.URL),
		"cover":      strings.TrimSpace(in.Cover),
		"transcript": in.Transcript,
	})
	if err != nil {
		return nil, err
	}
	existing, _ := s.repo.Items(ctx, lessonID)
	return s.AddItem(ctx, lessonID, kind, payload, len(existing))
}

// UpdateMedia rewrites an existing media item's payload (title, language, level,
// url, transcript) in place — used by the standalone media editor. Kind is not
// changed here (UpdateItem persists payload only).
func (s *Service) UpdateMedia(ctx context.Context, itemID string, in ImportMediaInput) (*coursedomain.LessonItem, error) {
	if strings.TrimSpace(itemID) == "" {
		return nil, apperr.ErrBadRequest
	}
	// URL is optional here: stories have none, and kind can't change in this
	// editor. Per-kind strictness (video/podcast must have a URL) is enforced
	// at creation and mirrored in the importer UI; this endpoint is admin-only.
	if strings.TrimSpace(in.Title) == "" || len(in.Transcript) == 0 {
		return nil, apperr.ErrBadRequest
	}
	lang := strings.TrimSpace(in.Language)
	if lang == "" {
		lang = "en"
	}
	payload, err := json.Marshal(map[string]any{
		"title":      strings.TrimSpace(in.Title),
		"language":   lang,
		"level":      strings.TrimSpace(in.CEFR),
		"url":        strings.TrimSpace(in.URL),
		"cover":      strings.TrimSpace(in.Cover),
		"transcript": in.Transcript,
	})
	if err != nil {
		return nil, err
	}
	return s.UpdateItem(ctx, itemID, payload, nil)
}

// ensureMediaLesson finds (or lazily creates) the hidden media bucket and
// returns a lesson id under it to attach media items to.
func (s *Service) ensureMediaLesson(ctx context.Context) (string, error) {
	courses, err := s.repo.ListAllCourses(ctx)
	if err != nil {
		return "", err
	}
	var courseID string
	for i := range courses {
		if courses[i].Title == MediaBucketTitle {
			courseID = courses[i].ID
			break
		}
	}
	if courseID == "" {
		c := &coursedomain.Course{Language: "en", CEFR: "A1", Title: MediaBucketTitle, Description: "System bucket for the media library.", Published: false}
		if err := s.repo.CreateCourse(ctx, c); err != nil {
			return "", err
		}
		courseID = c.ID
	}
	units, err := s.repo.Units(ctx, courseID)
	if err != nil {
		return "", err
	}
	var unitID string
	if len(units) > 0 {
		unitID = units[0].ID
	} else {
		u := &coursedomain.Unit{CourseID: courseID, Title: "Media"}
		if err := s.repo.CreateUnit(ctx, u); err != nil {
			return "", err
		}
		unitID = u.ID
	}
	lessons, err := s.repo.LessonsByUnit(ctx, unitID)
	if err != nil {
		return "", err
	}
	if len(lessons) > 0 {
		return lessons[0].ID, nil
	}
	l := &coursedomain.Lesson{UnitID: unitID, Title: "Media"}
	if err := s.repo.CreateLesson(ctx, l); err != nil {
		return "", err
	}
	return l.ID, nil
}
