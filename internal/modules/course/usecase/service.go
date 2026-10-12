// Package usecase holds the course application logic: browsing the learning tree
// and recording/summarizing progress. Depends only on the module's domain.
package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
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

// OnceRewarder is an optional extension of Rewarder: it grants a reward at
// most once per (user, event key), atomically, so retried or repeated requests
// can never double-award. Used when the injected rewarder supports it.
type OnceRewarder interface {
	AwardOnce(ctx context.Context, userID, eventKey string, xp int, reason string) (bool, error)
}

// Entitlements answers whether a learner may open a lesson right now (an
// active subscription, a class plan, or today's unlock). The billing module
// implements it; wiring injects it. Nil means no gating is configured.
type Entitlements interface {
	LessonAllowed(ctx context.Context, userID, lessonID string) (bool, error)
}

// SpeakingScores supplies the pronunciation score the SERVER recorded for a
// learner's latest attempt at a speak exercise, so speaking credit never
// trusts a score sent by the client. The pronunciation module implements it.
type SpeakingScores interface {
	LatestScore(ctx context.Context, userID, exerciseID string) (score int, ok bool, err error)
}

// Viewer is who asks for content: an anonymous visitor (empty UserID) or a
// signed-in learner. Admins see unpublished content and bypass entitlements.
type Viewer struct {
	UserID string
	Admin  bool
	// Account is the account type (learner | teacher | admin). Admins use the
	// learning app only to preview: answers are graded but nothing is
	// recorded. Teachers don't use it at all. Empty means learner.
	Account string
}

// Account types (mirroring authn).
const (
	AccountTeacher = "teacher"
	AccountAdmin   = "admin"
)

func (v Viewer) preview() bool { return v.Account == AccountAdmin }
func (v Viewer) learner() bool { return v.Account != AccountTeacher && v.Account != AccountAdmin }

// Service implements the course use cases.
type Service struct {
	courses      domain.CourseRepository
	progress     domain.ProgressRepository
	rewarder     Rewarder
	grammar      GrammarReviewer
	entitlements Entitlements
	speaking     SpeakingScores
	clock        func() time.Time
}

func NewService(courses domain.CourseRepository, progress domain.ProgressRepository, rewarder Rewarder, clock func() time.Time) *Service {
	return &Service{courses: courses, progress: progress, rewarder: rewarder, clock: clock}
}

// SetGrammarReviewer wires the srs enrollment port after construction (avoids a
// module dependency cycle, mirroring the other late-bound ports in app wiring).
func (s *Service) SetGrammarReviewer(g GrammarReviewer) { s.grammar = g }

// SetEntitlements wires the lesson-access port (billing) after construction.
func (s *Service) SetEntitlements(e Entitlements) { s.entitlements = e }

// SetSpeakingScores wires the server-side pronunciation score source.
func (s *Service) SetSpeakingScores(sc SpeakingScores) { s.speaking = sc }

// rewardOnce grants xp for an event at most once per user when the rewarder
// supports it (eventKey identifies the event, e.g. "lesson:<id>"). Rewards are
// best-effort: a failure never fails the learner's action, but it is logged
// instead of silently disappearing. Reports whether XP was granted.
func (s *Service) rewardOnce(ctx context.Context, userID, eventKey string, xp int, reason string) bool {
	if s.rewarder == nil {
		return false
	}
	if o, ok := s.rewarder.(OnceRewarder); ok {
		awarded, err := o.AwardOnce(ctx, userID, eventKey, xp, reason)
		if err != nil {
			slog.Warn("course: reward failed", "user", userID, "event", eventKey, "err", err)
			return false
		}
		return awarded
	}
	if err := s.rewarder.Award(ctx, userID, xp, reason); err != nil {
		slog.Warn("course: reward failed", "user", userID, "event", eventKey, "err", err)
		return false
	}
	return true
}

// visibleLesson loads a lesson the viewer may see at all: its course must be
// published (admins excepted). Unpublished content is reported as not found.
func (s *Service) visibleLesson(ctx context.Context, v Viewer, lessonID string) (*domain.Lesson, error) {
	l, err := s.courses.Lesson(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if !v.Admin {
		c, err := s.courses.Course(ctx, l.CourseID)
		if err != nil {
			return nil, err
		}
		if !c.Published {
			return nil, domain.ErrLessonNotFound
		}
	}
	return l, nil
}

// lessonOpen reports whether the viewer may use a lesson's content: free
// lessons are open to everyone; anything else needs a signed-in learner whom
// billing allows in (subscription, class plan or today's unlock). An access
// check that fails is returned as an error — never treated as open.
func (s *Service) lessonOpen(ctx context.Context, v Viewer, l *domain.Lesson) (bool, error) {
	if v.Admin || s.entitlements == nil {
		return true, nil
	}
	req, err := s.EffectiveLessonAccess(ctx, l.ID)
	if err != nil {
		return false, err
	}
	if req == domain.AccessFree {
		return true, nil
	}
	if v.UserID == "" {
		return false, nil
	}
	return s.entitlements.LessonAllowed(ctx, v.UserID, l.ID)
}

// completedLesson reports whether the user has already completed the lesson.
func (s *Service) completedLesson(ctx context.Context, userID string, l *domain.Lesson) (bool, error) {
	ids, err := s.progress.CompletedLessonIDs(ctx, userID, l.CourseID)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id == l.ID {
			return true, nil
		}
	}
	return false, nil
}

// practiceOpen is the access rule for answering a lesson's exercises: the
// lesson is open to the learner now, or they completed it earlier (reviewing
// what they already learned stays possible after a daily unlock expires).
func (s *Service) practiceOpen(ctx context.Context, v Viewer, l *domain.Lesson) (bool, error) {
	open, err := s.lessonOpen(ctx, v, l)
	if err != nil || open || v.UserID == "" {
		return open, err
	}
	return s.completedLesson(ctx, v.UserID, l)
}

// nonEmptyLessons returns the set of a course's lessons that have content.
// Lessons without items (e.g. imported checkpoints whose questions the source
// assembles at runtime) are hidden from learners and cannot be completed.
func (s *Service) nonEmptyLessons(ctx context.Context, courseID string) (map[string]bool, error) {
	counts, err := s.courses.LessonItemCounts(ctx, courseID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(counts))
	for id, n := range counts {
		if n > 0 {
			out[id] = true
		}
	}
	return out, nil
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

// LessonWithItems is a lesson with its content. Locked is set (and Items left
// empty) when the viewer may not open the lesson: the lesson itself stays
// visible so the client can offer an unlock, but its content never leaves.
type LessonWithItems struct {
	Lesson domain.Lesson
	Items  []ItemWithExercises
	Locked bool
	// Language is the course's target language (en, de, fr, tr…). The player
	// uses it for pronunciation and to file words into the right review deck.
	Language string
}

type ProgressSummary struct {
	TotalLessons     int
	CompletedLessons int
	Percent          int
	CompletedIDs     []string
}

// ListCourses returns published courses, optionally filtered by language/level.
func (s *Service) ListCourses(ctx context.Context, language, cefr string) ([]domain.Course, error) {
	courses, err := s.courses.ListCourses(ctx, language, cefr)
	if err != nil {
		return nil, err
	}
	// Learners only see courses of enabled languages; a disabled language's
	// courses stay in the catalog for admins (and for preparing content).
	enabled, err := s.enabledLanguages(ctx)
	if err != nil {
		return nil, err
	}
	out := courses[:0]
	for _, c := range courses {
		if enabled[c.Language] {
			out = append(out, c)
		}
	}
	return out, nil
}

// Languages returns the target languages learners can choose (enabled only),
// or every language when all is true.
func (s *Service) Languages(ctx context.Context, all bool) ([]domain.Language, error) {
	langs, err := s.courses.Languages(ctx)
	if err != nil {
		return nil, err
	}
	if all {
		return langs, nil
	}
	out := make([]domain.Language, 0, len(langs))
	for _, l := range langs {
		if l.Enabled {
			out = append(out, l)
		}
	}
	return out, nil
}

// LanguageEnabled reports whether learners may study code (used to validate a
// learner's chosen target language).
func (s *Service) LanguageEnabled(ctx context.Context, code string) (bool, error) {
	enabled, err := s.enabledLanguages(ctx)
	if err != nil {
		return false, err
	}
	return enabled[code], nil
}

func (s *Service) enabledLanguages(ctx context.Context) (map[string]bool, error) {
	langs, err := s.courses.Languages(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(langs))
	for _, l := range langs {
		if l.Enabled {
			m[l.Code] = true
		}
	}
	return m, nil
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
// a trailing unit with an empty ID rather than dropped. Unpublished courses
// are not found for learners, and lessons without any content are left out of
// the learner's path (admins see everything).
func (s *Service) GetCourse(ctx context.Context, v Viewer, id string) (*CourseWithLessons, error) {
	c, err := s.courses.Course(ctx, id)
	if err != nil {
		return nil, err
	}
	if !c.Published && !v.Admin {
		return nil, domain.ErrCourseNotFound
	}
	units, err := s.courses.Units(ctx, id)
	if err != nil {
		return nil, err
	}
	lessons, err := s.courses.Lessons(ctx, id)
	if err != nil {
		return nil, err
	}
	if !v.Admin {
		withContent, err := s.nonEmptyLessons(ctx, id)
		if err != nil {
			return nil, err
		}
		kept := lessons[:0:0]
		for _, l := range lessons {
			if withContent[l.ID] {
				kept = append(kept, l)
			}
		}
		lessons = kept
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
// The lesson's course must be published, and its content is only returned to
// a viewer who may open it (see lessonOpen); otherwise the lesson comes back
// Locked, without items.
func (s *Service) GetLesson(ctx context.Context, v Viewer, id string) (*LessonWithItems, error) {
	l, err := s.visibleLesson(ctx, v, id)
	if err != nil {
		return nil, err
	}
	open, err := s.lessonOpen(ctx, v, l)
	if err != nil {
		return nil, err
	}
	language := domain.DefaultLanguage
	if c, err := s.courses.Course(ctx, l.CourseID); err == nil && c.Language != "" {
		language = c.Language
	}
	if !open {
		return &LessonWithItems{Lesson: *l, Locked: true, Language: language}, nil
	}
	items, err := s.courses.Items(ctx, id)
	if err != nil {
		return nil, err
	}
	out := &LessonWithItems{Lesson: *l, Items: make([]ItemWithExercises, 0, len(items)), Language: language}
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

// CompletionResult is the outcome of completing a lesson.
type CompletionResult struct {
	AwardedXP bool // XP was granted (only for the first completion)
	Preview   bool // admin preview: nothing was recorded
	// Checkpoint results: a checkpoint completes only at CheckpointPassRatio
	// or better, scored on the latest attempt at each of its questions.
	Checkpoint bool
	Passed     bool
	Score      float64 // 0..1, checkpoints only
}

// CheckpointPassRatio is the share of a checkpoint's questions that must be
// correct (on their latest attempt) for it to count as passed — Busuu's 80%.
const CheckpointPassRatio = 0.8

// CompleteLesson records a lesson completion. The server decides whether the
// lesson counts as done: its course is published, the learner may open it (or
// completed it before), it has content, and every exercise was answered at
// least once. Speak exercises are optional — they need a microphone and a
// plan with pronunciation scoring, and the player lets a learner skip them —
// as are writing and roleplay items, which are not graded. Completion XP is
// awarded once per user and lesson, however often completion is requested.
func (s *Service) CompleteLesson(ctx context.Context, v Viewer, lessonID string) (*CompletionResult, error) {
	if v.Account == AccountTeacher {
		return nil, domain.ErrLearnerOnly
	}
	l, err := s.visibleLesson(ctx, v, lessonID)
	if err != nil {
		return nil, err
	}
	open, err := s.practiceOpen(ctx, v, l)
	if err != nil {
		return nil, err
	}
	if !open {
		return nil, domain.ErrLessonLocked
	}
	items, err := s.courses.Items(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, domain.ErrLessonEmpty
	}
	// Admin preview: the lesson can be finished without answering everything,
	// and nothing is recorded or awarded.
	if v.preview() {
		return &CompletionResult{Passed: true, Preview: true}, nil
	}
	var required []string
	for _, it := range items {
		if it.Kind != domain.KindExercise {
			continue
		}
		exs, err := s.courses.Exercises(ctx, it.ID)
		if err != nil {
			return nil, err
		}
		for _, e := range exs {
			if e.Type != TypeSpeak {
				required = append(required, e.ID)
			}
		}
	}
	if len(required) > 0 {
		done, err := s.progress.AttemptedExercises(ctx, v.UserID, required)
		if err != nil {
			return nil, err
		}
		for _, id := range required {
			if !done[id] {
				return nil, domain.ErrLessonIncomplete
			}
		}
	}
	res := &CompletionResult{Passed: true}
	if l.Kind == domain.LessonKindCheckpoint && len(required) > 0 {
		scores, err := s.progress.LatestExerciseScores(ctx, v.UserID, required)
		if err != nil {
			return nil, err
		}
		correct := 0.0
		for _, id := range required {
			correct += scores[id]
		}
		res.Checkpoint = true
		res.Score = correct / float64(len(required))
		// Below the pass mark nothing is recorded and nothing is awarded; the
		// learner retakes the checkpoint and the latest answers count.
		if res.Score < CheckpointPassRatio {
			res.Passed = false
			return res, nil
		}
	}
	already, err := s.completedLesson(ctx, v.UserID, l)
	if err != nil {
		return nil, err
	}
	if err := s.progress.Record(ctx, domain.ProgressEntry{
		UserID:     v.UserID,
		EntityType: domain.EntityLesson,
		EntityID:   lessonID,
		Status:     domain.StatusCompleted,
		OccurredAt: s.clock(),
	}); err != nil {
		return nil, err
	}
	if !already {
		res.AwardedXP = s.rewardOnce(ctx, v.UserID, "lesson:"+lessonID, XPLessonComplete, "lesson_complete")
	}
	return res, nil
}

// GradeResult is the outcome of grading an exercise attempt. Solution carries
// the correct answer for post-attempt feedback (Busuu-style "here's the answer").
type GradeResult struct {
	Correct   bool
	Score     float64
	Solution  json.RawMessage
	FirstTry  bool // this was the user's first attempt at the exercise
	AwardedXP bool // XP was granted (first-attempt-correct only)
	Preview   bool // admin preview: graded, nothing recorded
}

// GradeExercise grades a learner's answer SERVER-SIDE for any supported type
// (the answer key never leaves the server), records the attempt, awards XP on a
// first-attempt-correct answer, and returns the result plus the correct
// solution. The exercise's lesson must be published and open to the learner.
// A missing or malformed answer is rejected (ErrInvalidAnswer) without being
// recorded. Speak exercises are graded from the server's own pronunciation
// assessment, never from a score the client sends.
func (s *Service) GradeExercise(ctx context.Context, v Viewer, exerciseID string, answer json.RawMessage) (*GradeResult, error) {
	if v.Account == AccountTeacher {
		return nil, domain.ErrLearnerOnly
	}
	ex, err := s.courses.Exercise(ctx, exerciseID)
	if err != nil {
		return nil, err
	}
	lessonID, err := s.courses.ExerciseLessonID(ctx, exerciseID)
	if err != nil {
		return nil, err
	}
	l, err := s.visibleLesson(ctx, v, lessonID)
	if err != nil {
		if err == domain.ErrLessonNotFound {
			return nil, domain.ErrExerciseNotFound
		}
		return nil, err
	}
	open, err := s.practiceOpen(ctx, v, l)
	if err != nil {
		return nil, err
	}
	if !open {
		return nil, domain.ErrLessonLocked
	}
	var correct bool
	var solution json.RawMessage
	if ex.Type == TypeSpeak && v.preview() {
		return &GradeResult{Correct: true, Score: 1, Preview: true}, nil
	}
	if ex.Type == TypeSpeak {
		if s.speaking == nil {
			return nil, domain.ErrNoAssessment
		}
		score, ok, err := s.speaking.LatestScore(ctx, v.UserID, exerciseID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, domain.ErrNoAssessment
		}
		correct, solution, err = gradeSpeak(ex, score)
		if err != nil {
			return nil, err
		}
	} else if correct, solution, err = grade(ex, answer); err != nil {
		return nil, err
	}
	// Admin preview: grade and show the result, record nothing (no attempt,
	// no XP, no review enrolment).
	if v.preview() {
		res := &GradeResult{Correct: correct, Solution: solution, Preview: true}
		if correct {
			res.Score = 1
		}
		return res, nil
	}
	// Count prior attempts BEFORE recording this one: XP is granted only for a
	// first-attempt-correct answer. A wrong first attempt earns nothing, and any
	// later correction (e.g. in the end-of-lesson review) sees a prior attempt
	// and stays unrewarded — so a wrong answer never earns points, and correct
	// answers can't be re-farmed by re-submitting.
	prior, err := s.progress.ExerciseAttempts(ctx, v.UserID, exerciseID)
	if err != nil {
		return nil, err
	}
	res := &GradeResult{Correct: correct, Solution: solution, FirstTry: prior == 0}
	if correct {
		res.Score = 1
	}
	if err := s.progress.Record(ctx, domain.ProgressEntry{
		UserID:     v.UserID,
		EntityType: domain.EntityExercise,
		EntityID:   exerciseID,
		Status:     domain.StatusCompleted,
		Score:      &res.Score,
		OccurredAt: s.clock(),
	}); err != nil {
		return nil, err
	}
	if correct && prior == 0 {
		res.AwardedXP = s.rewardOnce(ctx, v.UserID, "exercise:"+exerciseID, XPExerciseCorrect, "exercise_correct")
	}
	// A missed grammar-tagged exercise enrolls its rule into spaced repetition so
	// the learner revisits it. Best-effort: never fails the grade.
	if !correct {
		if pattern := grammarPattern(ex); pattern != "" && s.grammar != nil {
			_ = s.grammar.EnrollGrammar(ctx, v.UserID, "en", pattern)
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

// CourseProgress summarizes how far a user has gotten in a course. Only
// lessons with content count, matching the learner's path (GetCourse).
func (s *Service) CourseProgress(ctx context.Context, userID, courseID string) (*ProgressSummary, error) {
	if _, err := s.courses.Course(ctx, courseID); err != nil {
		return nil, err
	}
	withContent, err := s.nonEmptyLessons(ctx, courseID)
	if err != nil {
		return nil, err
	}
	all, err := s.progress.CompletedLessonIDs(ctx, userID, courseID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(all))
	for _, id := range all {
		if withContent[id] {
			ids = append(ids, id)
		}
	}
	total, done := len(withContent), len(ids)
	percent := 0
	if total > 0 {
		percent = done * 100 / total
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

// LessonRequirement resolves the access requirement of a lesson a learner can
// reach, for billing: a missing lesson, or one in an unpublished course, is not
// found — so no free unlock or gems are ever spent on it.
func (s *Service) LessonRequirement(ctx context.Context, lessonID string) (string, error) {
	l, err := s.visibleLesson(ctx, Viewer{}, lessonID)
	if err != nil {
		return "", err
	}
	return s.EffectiveLessonAccess(ctx, l.ID)
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
	// The free tier: every course's first chapter is open to everyone; the rest
	// needs the learning subscription. Explicit rules above always win.
	if s.inFirstChapter(ctx, l) {
		return domain.AccessFree, nil
	}
	return domain.AccessDefault, nil
}

// inFirstChapter reports whether the lesson belongs to its course's first
// chapter (the unit with the lowest position). Lessons outside any chapter are
// not part of the free tier.
func (s *Service) inFirstChapter(ctx context.Context, l *domain.Lesson) bool {
	if l.UnitID == "" {
		return false
	}
	units, err := s.courses.Units(ctx, l.CourseID)
	if err != nil || len(units) == 0 {
		return false
	}
	first := units[0]
	for _, u := range units[1:] {
		if u.Position < first.Position {
			first = u
		}
	}
	return first.ID == l.UnitID
}

// quickPracticeAccessChecks bounds how many not-yet-completed lessons one quick
// practice request checks against billing (each check is a few queries).
const quickPracticeAccessChecks = 30

// QuickPractice returns up to n gradeable exercises drawn at random from the
// published lessons this learner may practice — lessons open to them now, or
// completed earlier — so every exercise it serves can also be graded. A light,
// lesson-independent review session. Speak exercises are excluded because they
// require the microphone flow.
func (s *Service) QuickPractice(ctx context.Context, userID string, n int) ([]domain.Exercise, error) {
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
		"fill_gaps": true,
	}
	courses, err := s.courses.ListCourses(ctx, "", "")
	if err != nil {
		return nil, err
	}
	type candidate struct {
		lesson domain.Lesson
		done   bool
	}
	var cands []candidate
	for _, c := range courses {
		lessons, lerr := s.courses.Lessons(ctx, c.ID)
		if lerr != nil {
			continue
		}
		done := map[string]bool{}
		if ids, err := s.progress.CompletedLessonIDs(ctx, userID, c.ID); err == nil {
			for _, id := range ids {
				done[id] = true
			}
		}
		for _, l := range lessons {
			cands = append(cands, candidate{lesson: l, done: done[l.ID]})
		}
	}
	rand.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
	v := Viewer{UserID: userID}
	checks := 0
	var pool []domain.Exercise
	for _, c := range cands {
		if len(pool) >= n*3 {
			break // enough variety
		}
		if !c.done {
			if checks >= quickPracticeAccessChecks {
				continue
			}
			checks++
			if open, err := s.lessonOpen(ctx, v, &c.lesson); err != nil || !open {
				continue
			}
		}
		items, ierr := s.courses.Items(ctx, c.lesson.ID)
		if ierr != nil {
			continue
		}
		for _, it := range items {
			if it.Kind != domain.KindExercise {
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
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	if n > len(pool) {
		n = len(pool)
	}
	return pool[:n], nil
}
