// Package usecase holds the course application logic: browsing the learning tree
// and recording/summarizing progress. Depends only on the module's domain.
package usecase

import (
	"context"
	"encoding/json"
	"math/rand"
	"sort"
	"time"

	"github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// XP awards for learning actions.
const (
	XPLessonComplete  = 20
	XPExerciseCorrect = 10
)

// Rewarder is the port the course module calls to grant XP. The gamification
// module implements it; wiring injects it. Declared here so course depends on
// no other module. It is optional — a nil Rewarder simply skips rewards.
type Rewarder interface {
	Award(ctx context.Context, userID string, xp int, reason string) error
}

// GrammarReviewer is the port the course module calls to enroll a grammar rule
// into spaced repetition when a learner misses a grammar-tagged exercise. The
// srs module implements it; wiring injects it. Optional — nil skips enrollment.
type GrammarReviewer interface {
	EnrollGrammar(ctx context.Context, userID, language, pattern string) error
}

// Service implements the course use cases.
type Service struct {
	courses  domain.CourseRepository
	progress domain.ProgressRepository
	rewarder Rewarder
	grammar  GrammarReviewer
	clock    func() time.Time
}

func NewService(courses domain.CourseRepository, progress domain.ProgressRepository, rewarder Rewarder, clock func() time.Time) *Service {
	return &Service{courses: courses, progress: progress, rewarder: rewarder, clock: clock}
}

// SetGrammarReviewer wires the srs enrollment port after construction (avoids a
// module dependency cycle, mirroring the other late-bound ports in app wiring).
func (s *Service) SetGrammarReviewer(g GrammarReviewer) { s.grammar = g }

func (s *Service) reward(ctx context.Context, userID string, xp int, reason string) {
	if s.rewarder != nil {
		_ = s.rewarder.Award(ctx, userID, xp, reason) // best-effort; never fails the action
	}
}

// --- read models (aggregates returned to the delivery layer) ---

// CourseWithLessons is a course with its chapter hierarchy. Units are in
// position order and each holds its lessons in position order; Lessons is the
// same lessons flattened in that global learning order (unit by unit), for
// callers that only need a sequence.
type CourseWithLessons struct {
	Course  domain.Course
	Units   []UnitWithLessons
	Lessons []domain.Lesson
}

// UnitWithLessons is one unit (chapter) of a course with its ordered lessons.
type UnitWithLessons struct {
	Unit    domain.Unit
	Lessons []domain.Lesson
}

type ItemWithExercises struct {
	Item      domain.LessonItem
	Exercises []domain.Exercise
}

type LessonWithItems struct {
	Lesson domain.Lesson
	Items  []ItemWithExercises
}

type ProgressSummary struct {
	TotalLessons     int
	CompletedLessons int
	Percent          int
	CompletedIDs     []string
}

// ListCourses returns published courses, optionally filtered by language/level.
func (s *Service) ListCourses(ctx context.Context, language, cefr string) ([]domain.Course, error) {
	return s.courses.ListCourses(ctx, language, cefr)
}

// PracticeExercises returns up to n exercises for a quick standalone practice
// session, drawn only from lessons the learner can access (their completed
// lessons plus the always-free first lesson) and shuffled. Prompts are returned
// raw; the REST layer sanitizes them via exerciseView before sending.
func (s *Service) PracticeExercises(ctx context.Context, userID, courseID string, n int) ([]domain.Exercise, error) {
	if n <= 0 || n > 20 {
		n = 5
	}
	if courseID == "" {
		courses, err := s.courses.ListCourses(ctx, "", "")
		if err != nil {
			return nil, err
		}
		if len(courses) == 0 {
			return nil, nil
		}
		courseID = courses[0].ID
	}
	lessons, err := s.courses.Lessons(ctx, courseID)
	if err != nil {
		return nil, err
	}
	accessible := map[string]bool{}
	if ids, err := s.progress.CompletedLessonIDs(ctx, userID, courseID); err == nil {
		for _, id := range ids {
			accessible[id] = true
		}
	}
	if len(lessons) > 0 {
		accessible[lessons[0].ID] = true // first lesson is always free
	}
	var pool []domain.Exercise
	for _, l := range lessons {
		if !accessible[l.ID] {
			continue
		}
		items, err := s.courses.Items(ctx, l.ID)
		if err != nil {
			continue
		}
		for _, it := range items {
			es, err := s.courses.Exercises(ctx, it.ID)
			if err != nil {
				continue
			}
			pool = append(pool, es...)
		}
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if len(pool) > n {
		pool = pool[:n]
	}
	return pool, nil
}

// GetCourse returns a course with its units (chapters) and their lessons, both
// ordered by position. Lessons whose unit is unknown (legacy rows) are kept in
// a trailing unit with an empty ID rather than dropped.
func (s *Service) GetCourse(ctx context.Context, id string) (*CourseWithLessons, error) {
	c, err := s.courses.Course(ctx, id)
	if err != nil {
		return nil, err
	}
	units, err := s.courses.Units(ctx, id)
	if err != nil {
		return nil, err
	}
	lessons, err := s.courses.Lessons(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &CourseWithLessons{Course: *c, Units: make([]UnitWithLessons, len(units))}
	at := make(map[string]int, len(units))
	for i, u := range units {
		out.Units[i] = UnitWithLessons{Unit: u}
		at[u.ID] = i
	}
	var orphans []domain.Lesson
	for _, l := range lessons {
		if i, ok := at[l.UnitID]; ok {
			out.Units[i].Lessons = append(out.Units[i].Lessons, l)
		} else {
			orphans = append(orphans, l)
		}
	}
	for _, u := range out.Units {
		sort.SliceStable(u.Lessons, func(a, b int) bool { return u.Lessons[a].Position < u.Lessons[b].Position })
		out.Lessons = append(out.Lessons, u.Lessons...)
	}
	if len(orphans) > 0 {
		out.Units = append(out.Units, UnitWithLessons{Lessons: orphans})
		out.Lessons = append(out.Lessons, orphans...)
	}
	return out, nil
}

// GetLesson returns a lesson with its items (ordered) and each item's exercises.
func (s *Service) GetLesson(ctx context.Context, id string) (*LessonWithItems, error) {
	l, err := s.courses.Lesson(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := s.courses.Items(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &LessonWithItems{Lesson: *l, Items: make([]ItemWithExercises, 0, len(items))}
	items = s.localizeItems(ctx, items)
	for _, it := range items {
		var exs []domain.Exercise
		if it.Kind == domain.KindExercise {
			if exs, err = s.courses.Exercises(ctx, it.ID); err != nil {
				return nil, err
			}
		}
		out.Items = append(out.Items, ItemWithExercises{Item: it, Exercises: exs})
	}
	return out, nil
}

// localizeItems adds stored translations of explanatory text (grammar tips) to
// item payloads, for every locale that has one; the learner client picks its
// own. Best-effort: on a lookup error the items are served untranslated.
func (s *Service) localizeItems(ctx context.Context, items []domain.LessonItem) []domain.LessonItem {
	var hashes []string
	for _, it := range items {
		for _, ls := range domain.LocalizableStrings(it.Kind, it.Payload) {
			hashes = append(hashes, domain.SourceHash(ls.Text))
		}
	}
	if len(hashes) == 0 {
		return items
	}
	stored, err := s.courses.ContentTranslations(ctx, hashes)
	if err != nil || len(stored) == 0 {
		return items
	}
	tr := map[string]map[string]string{}
	for _, t := range stored {
		if tr[t.SourceHash] == nil {
			tr[t.SourceHash] = map[string]string{}
		}
		tr[t.SourceHash][t.Locale] = t.Text
	}
	out := make([]domain.LessonItem, len(items))
	for i, it := range items {
		it.Payload = domain.LocalizePayload(it.Kind, it.Payload, tr)
		out[i] = it
	}
	return out
}

// CompleteLesson records a lesson completion for the user (validates existence).
func (s *Service) CompleteLesson(ctx context.Context, userID, lessonID string) error {
	if _, err := s.courses.Lesson(ctx, lessonID); err != nil {
		return err
	}
	if err := s.progress.Record(ctx, domain.ProgressEntry{
		UserID:     userID,
		EntityType: domain.EntityLesson,
		EntityID:   lessonID,
		Status:     domain.StatusCompleted,
		OccurredAt: s.clock(),
	}); err != nil {
		return err
	}
	s.reward(ctx, userID, XPLessonComplete, "lesson_complete")
	return nil
}

// GradeResult is the outcome of grading an exercise attempt. Solution carries
// the correct answer for post-attempt feedback (Busuu-style "here's the answer").
type GradeResult struct {
	Correct   bool
	Score     float64
	Solution  json.RawMessage
	FirstTry  bool // this was the user's first attempt at the exercise
	AwardedXP bool // XP was granted (first-attempt-correct only)
}

// GradeExercise grades a learner's answer SERVER-SIDE for any supported type
// (the answer key never leaves the server), records the attempt, awards XP on a
// correct answer, and returns the result plus the correct solution.
func (s *Service) GradeExercise(ctx context.Context, userID, exerciseID string, answer json.RawMessage) (*GradeResult, error) {
	ex, err := s.courses.Exercise(ctx, exerciseID)
	if err != nil {
		return nil, err
	}
	// Count prior attempts BEFORE recording this one: XP is granted only for a
	// first-attempt-correct answer. A wrong first attempt earns nothing, and any
	// later correction (e.g. in the end-of-lesson review) sees a prior attempt
	// and stays unrewarded — so a wrong answer never earns points, and correct
	// answers can't be re-farmed by re-submitting.
	prior, err := s.progress.ExerciseAttempts(ctx, userID, exerciseID)
	if err != nil {
		return nil, err
	}
	correct, solution := grade(ex, answer)
	res := &GradeResult{Correct: correct, Solution: solution, FirstTry: prior == 0}
	if correct {
		res.Score = 1
	}
	if err := s.progress.Record(ctx, domain.ProgressEntry{
		UserID:     userID,
		EntityType: domain.EntityExercise,
		EntityID:   exerciseID,
		Status:     domain.StatusCompleted,
		Score:      &res.Score,
		OccurredAt: s.clock(),
	}); err != nil {
		return nil, err
	}
	if correct && prior == 0 {
		s.reward(ctx, userID, XPExerciseCorrect, "exercise_correct")
		res.AwardedXP = true
	}
	// A missed grammar-tagged exercise enrolls its rule into spaced repetition so
	// the learner revisits it. Best-effort: never fails the grade.
	if !correct {
		if pattern := grammarPattern(ex); pattern != "" && s.grammar != nil {
			_ = s.grammar.EnrollGrammar(ctx, userID, "en", pattern)
		}
	}
	return res, nil
}

// grammarPattern extracts the optional grammar rule a prompt is tagged with
// (e.g. {"grammar":"to be"}); empty when the exercise isn't grammar-focused.
func grammarPattern(ex *domain.Exercise) string {
	if ex == nil || len(ex.Prompt) == 0 {
		return ""
	}
	var p struct {
		Grammar string `json:"grammar"`
	}
	if err := json.Unmarshal(ex.Prompt, &p); err != nil {
		return ""
	}
	return p.Grammar
}

// CourseProgress summarizes how far a user has gotten in a course.
func (s *Service) CourseProgress(ctx context.Context, userID, courseID string) (*ProgressSummary, error) {
	if _, err := s.courses.Course(ctx, courseID); err != nil {
		return nil, err
	}
	total, err := s.courses.LessonCount(ctx, courseID)
	if err != nil {
		return nil, err
	}
	done, err := s.progress.CompletedLessons(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	percent := 0
	if total > 0 {
		percent = done * 100 / total
	}
	ids, err := s.progress.CompletedLessonIDs(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	return &ProgressSummary{TotalLessons: total, CompletedLessons: done, Percent: percent, CompletedIDs: ids}, nil
}

// MediaLibrary lists server-hosted media (podcasts and films with transcripts),
// for the learner-facing media library. Public.
func (s *Service) MediaLibrary(ctx context.Context, kind, language string, limit, offset int) ([]domain.LessonItem, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.courses.MediaItems(ctx, kind, language, limit, offset)
}

// validAccess reports whether r is a concrete access requirement.
func validAccess(r string) bool {
	return r == domain.AccessFree || r == domain.AccessCourses || r == domain.AccessClasses
}

// SetAccess pins (or, when requirement is "inherit"/empty, clears) the access
// requirement for a course, unit, or lesson.
func (s *Service) SetAccess(ctx context.Context, entityType, entityID, requirement string) error {
	switch entityType {
	case "course", "unit", "lesson":
	default:
		return apperr.ErrBadRequest
	}
	if requirement == "" || requirement == "inherit" {
		return s.courses.DeleteAccessRule(ctx, entityType, entityID)
	}
	if !validAccess(requirement) {
		return apperr.ErrBadRequest
	}
	return s.courses.SetAccessRule(ctx, domain.AccessRule{EntityType: entityType, EntityID: entityID, Requirement: requirement})
}

// AccessRules returns all explicit rules, for the admin console.
func (s *Service) AccessRules(ctx context.Context) ([]domain.AccessRule, error) {
	return s.courses.ListAccessRules(ctx)
}

// ruleFor returns the explicit requirement for an entity, or "" if none.
func (s *Service) ruleFor(ctx context.Context, entityType, entityID string) string {
	if entityID == "" {
		return ""
	}
	if ar, _ := s.courses.GetAccessRule(ctx, entityType, entityID); ar != nil {
		return ar.Requirement
	}
	return ""
}

// EffectiveLessonAccess resolves a lesson's access requirement by walking up the
// tree: lesson -> unit -> course -> default.
func (s *Service) EffectiveLessonAccess(ctx context.Context, lessonID string) (string, error) {
	l, err := s.courses.Lesson(ctx, lessonID)
	if err != nil {
		return "", err
	}
	if l == nil {
		return domain.AccessDefault, nil
	}
	if r := s.ruleFor(ctx, "lesson", l.ID); r != "" {
		return r, nil
	}
	if r := s.ruleFor(ctx, "unit", l.UnitID); r != "" {
		return r, nil
	}
	if r := s.ruleFor(ctx, "course", l.CourseID); r != "" {
		return r, nil
	}
	return domain.AccessDefault, nil
}

// QuickPractice returns up to n gradeable exercises drawn at random from across
// the published courses — a light, lesson-independent review session. Speak
// exercises are excluded because they require the microphone flow.
func (s *Service) QuickPractice(ctx context.Context, n int) ([]domain.Exercise, error) {
	if n <= 0 {
		n = 5
	}
	if n > 20 {
		n = 20
	}
	gradeable := map[string]bool{
		"mcq": true, "listen_choose": true, "true_false": true,
		"fill_blank": true, "dictation": true, "reorder": true,
		"sentence_builder": true, "match": true, "match_image": true,
	}
	courses, err := s.courses.ListCourses(ctx, "", "")
	if err != nil {
		return nil, err
	}
	var pool []domain.Exercise
	for _, c := range courses {
		lessons, lerr := s.courses.Lessons(ctx, c.ID)
		if lerr != nil {
			continue
		}
		for _, l := range lessons {
			items, ierr := s.courses.Items(ctx, l.ID)
			if ierr != nil {
				continue
			}
			for _, it := range items {
				if it.Kind != "exercise" {
					continue
				}
				exs, eerr := s.courses.Exercises(ctx, it.ID)
				if eerr != nil {
					continue
				}
				for _, e := range exs {
					if gradeable[e.Type] {
						pool = append(pool, e)
					}
				}
			}
		}
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if n > len(pool) {
		n = len(pool)
	}
	return pool[:n], nil
}
