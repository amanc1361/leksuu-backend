package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/modules/course/repo"
	"github.com/leksuu/platform/internal/modules/course/usecase"
)

func newService() *usecase.Service {
	courses := repo.NewMemoryCourseRepo()
	return usecase.NewService(courses, repo.NewMemoryProgressRepo(courses.LessonCourseID), nil, time.Now)
}

func learner(id string) usecase.Viewer { return usecase.Viewer{UserID: id} }

// answerAll answers every non-speak exercise of a lesson (correctly or not:
// completion only requires an attempt).
func answerAll(t *testing.T, s *usecase.Service, v usecase.Viewer, lessonID string) {
	t.Helper()
	ctx := context.Background()
	lwi, err := s.GetLesson(ctx, v, lessonID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range lwi.Items {
		for _, e := range it.Exercises {
			var ans string
			switch e.Type {
			case "mcq", "listen_choose":
				ans = `{"choice":0}`
			case "fill_blank", "dictation":
				ans = `{"text":"x"}`
			case "true_false":
				ans = `{"value":true}`
			case "reorder", "sentence_builder":
				ans = `{"order":["x"]}`
			case "match", "match_image":
				ans = `{"pairs":[["a","b"]]}`
			default:
				continue
			}
			if _, err := s.GradeExercise(ctx, v, e.ID, json.RawMessage(ans)); err != nil {
				t.Fatalf("answer %s: %v", e.ID, err)
			}
		}
	}
}

func TestBrowseTreeOrderedByPosition(t *testing.T) {
	ctx := context.Background()
	s := newService()

	courses, err := s.ListCourses(ctx, "en", "A1")
	if err != nil || len(courses) != 1 {
		t.Fatalf("list: %d courses, err=%v", len(courses), err)
	}
	cid := courses[0].ID

	// filter that matches nothing
	if other, _ := s.ListCourses(ctx, "fr", ""); len(other) != 0 {
		t.Fatalf("expected no French courses, got %d", len(other))
	}

	cwl, err := s.GetCourse(ctx, usecase.Viewer{}, cid)
	if err != nil || len(cwl.Lessons) < 2 {
		t.Fatalf("course: %v lessons=%d", err, len(cwl.Lessons))
	}
	if cwl.Lessons[0].Position > cwl.Lessons[1].Position {
		t.Fatal("lessons not ordered by position")
	}

	lwi, err := s.GetLesson(ctx, usecase.Viewer{}, cwl.Lessons[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(lwi.Items); i++ {
		if lwi.Items[i-1].Item.Position > lwi.Items[i].Item.Position {
			t.Fatal("items not ordered by position")
		}
	}
	// the exercise item must carry its exercise; answer keys are stripped at the
	// delivery layer, but the use case still loads them.
	var foundExercise bool
	for _, it := range lwi.Items {
		if len(it.Exercises) > 0 {
			foundExercise = true
		}
	}
	if !foundExercise {
		t.Fatal("expected an exercise in the first lesson")
	}
}

func TestCourseProgress(t *testing.T) {
	ctx := context.Background()
	s := newService()
	cid := "course-en-a1"
	user := learner("user-1")

	if _, err := s.GetCourse(ctx, usecase.Viewer{}, "nope"); err == nil {
		t.Fatal("expected not found")
	}

	sum, err := s.CourseProgress(ctx, user.UserID, cid)
	if err != nil || sum.Percent != 0 {
		t.Fatalf("initial progress: %+v err=%v", sum, err)
	}

	answerAll(t, s, user, "lesson-greetings")
	if _, err := s.CompleteLesson(ctx, user, "lesson-greetings"); err != nil {
		t.Fatal(err)
	}
	// completing the same lesson twice must not double-count
	_, _ = s.CompleteLesson(ctx, user, "lesson-greetings")

	sum, _ = s.CourseProgress(ctx, user.UserID, cid)
	if sum.CompletedLessons != 1 || sum.TotalLessons < 2 || sum.Percent != 100*sum.CompletedLessons/sum.TotalLessons {
		t.Fatalf("after one completion: %+v", sum)
	}

	if _, err := s.CompleteLesson(ctx, user, "missing-lesson"); err == nil {
		t.Fatal("completing a missing lesson should error")
	}

	// N08: a lesson with no content (lesson-introductions has no items) cannot
	// be completed and must not count toward progress (or grant rewards).
	if _, err := s.CompleteLesson(ctx, user, "lesson-empty-cp"); err == nil {
		t.Fatal("completing an empty lesson should error")
	}
	sum, _ = s.CourseProgress(ctx, user.UserID, cid)
	if sum.CompletedLessons != 1 {
		t.Fatalf("empty lesson must not count as completed: %+v", sum)
	}
}

func TestEffectiveLessonAccessResolution(t *testing.T) {
	r := repo.NewMemoryCourseRepo()
	svc := usecase.NewService(r, repo.NewMemoryProgressRepo(r.LessonCourseID), nil, time.Now)
	ctx := context.Background()

	courses, _ := r.ListCourses(ctx, "", "")
	if len(courses) == 0 {
		t.Skip("no seeded courses")
	}
	cid := courses[0].ID
	lessons, _ := r.Lessons(ctx, cid)
	if len(lessons) == 0 {
		t.Skip("no seeded lessons")
	}
	lid := lessons[0].ID

	if a, _ := svc.EffectiveLessonAccess(ctx, lid); a != domain.AccessDefault {
		t.Fatalf("default = %s, want %s", a, domain.AccessDefault)
	}
	_ = svc.SetAccess(ctx, "course", cid, domain.AccessClasses)
	if a, _ := svc.EffectiveLessonAccess(ctx, lid); a != domain.AccessClasses {
		t.Fatalf("course rule = %s, want classes", a)
	}
	_ = svc.SetAccess(ctx, "lesson", lid, domain.AccessFree)
	if a, _ := svc.EffectiveLessonAccess(ctx, lid); a != domain.AccessFree {
		t.Fatalf("lesson override = %s, want free", a)
	}
	_ = svc.SetAccess(ctx, "lesson", lid, "inherit")
	if a, _ := svc.EffectiveLessonAccess(ctx, lid); a != domain.AccessClasses {
		t.Fatalf("after clear = %s, want classes", a)
	}
}

type countRewarder struct {
	awards int
	xp     int
}

func (c *countRewarder) Award(_ context.Context, _ string, xp int, _ string) error {
	c.awards++
	c.xp += xp
	return nil
}

// XP is granted only for a first-attempt-correct answer: a wrong answer earns
// nothing, a later correction stays unrewarded, and correct answers can't be
// re-farmed by re-submitting.
func TestExerciseXPFirstAttemptOnly(t *testing.T) {
	ctx := context.Background()
	courses := repo.NewMemoryCourseRepo()
	rw := &countRewarder{}
	s := usecase.NewService(courses, repo.NewMemoryProgressRepo(courses.LessonCourseID), rw, time.Now)

	// First attempt wrong → no XP.
	r, err := s.GradeExercise(ctx, learner("u1"), "ex-numbers-1", json.RawMessage(`{"choice":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Correct || r.AwardedXP || rw.awards != 0 {
		t.Fatalf("wrong first attempt must earn nothing: correct=%v awarded=%v awards=%d", r.Correct, r.AwardedXP, rw.awards)
	}

	// Same user corrects it on a later attempt → still no XP (was wrong first).
	r, err = s.GradeExercise(ctx, learner("u1"), "ex-numbers-1", json.RawMessage(`{"choice":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Correct || r.FirstTry || r.AwardedXP || rw.awards != 0 {
		t.Fatalf("corrected-after-wrong must not earn XP: %+v awards=%d", r, rw.awards)
	}

	// A different user gets it right first try → XP awarded exactly once.
	r, err = s.GradeExercise(ctx, learner("u2"), "ex-numbers-1", json.RawMessage(`{"choice":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Correct || !r.FirstTry || !r.AwardedXP {
		t.Fatalf("first-try-correct must award XP: %+v", r)
	}
	if rw.awards != 1 || rw.xp != usecase.XPExerciseCorrect {
		t.Fatalf("want 1 award of %d XP, got awards=%d xp=%d", usecase.XPExerciseCorrect, rw.awards, rw.xp)
	}

	// Re-submitting an already-correct exercise must not re-award (no farming).
	if _, err := s.GradeExercise(ctx, learner("u2"), "ex-numbers-1", json.RawMessage(`{"choice":0}`)); err != nil {
		t.Fatal(err)
	}
	if rw.awards != 1 {
		t.Fatalf("re-submit must not re-award, got %d awards", rw.awards)
	}
}

// An invalid answer is rejected and NOT recorded: it neither earns credit nor
// burns the learner's first attempt.
func TestInvalidAnswerIsNotRecorded(t *testing.T) {
	ctx := context.Background()
	courses := repo.NewMemoryCourseRepo()
	rw := &countRewarder{}
	s := usecase.NewService(courses, repo.NewMemoryProgressRepo(courses.LessonCourseID), rw, time.Now)

	if _, err := s.GradeExercise(ctx, learner("u1"), "ex-numbers-1", json.RawMessage(`{}`)); !errors.Is(err, usecase.ErrInvalidAnswer) {
		t.Fatalf("empty answer: want ErrInvalidAnswer, got %v", err)
	}
	r, err := s.GradeExercise(ctx, learner("u1"), "ex-numbers-1", json.RawMessage(`{"choice":0}`))
	if err != nil || !r.Correct || !r.FirstTry || !r.AwardedXP {
		t.Fatalf("the first real answer is still the first try: %+v err=%v", r, err)
	}
}

// fakeEntitlements allows only the listed (user, lesson) pairs.
type fakeEntitlements map[string]bool

func (f fakeEntitlements) LessonAllowed(_ context.Context, userID, lessonID string) (bool, error) {
	return f[userID+"|"+lessonID], nil
}

// onceRewarder implements both Rewarder and OnceRewarder.
type onceRewarder struct {
	countRewarder
	seen map[string]bool
}

func (o *onceRewarder) AwardOnce(ctx context.Context, userID, key string, xp int, reason string) (bool, error) {
	if o.seen == nil {
		o.seen = map[string]bool{}
	}
	if o.seen[userID+"|"+key] {
		return false, nil
	}
	o.seen[userID+"|"+key] = true
	return true, o.Award(ctx, userID, xp, reason)
}

func gatedService(t *testing.T, ent fakeEntitlements, rw usecase.Rewarder) (*usecase.Service, *repo.MemoryCourseRepo) {
	t.Helper()
	courses := repo.NewMemoryCourseRepo()
	s := usecase.NewService(courses, repo.NewMemoryProgressRepo(courses.LessonCourseID), rw, time.Now)
	s.SetEntitlements(ent)
	return s, courses
}

// Locked lessons keep their content on the server: no items for anonymous or
// non-entitled viewers, and no grading or completion either.
func TestLockedLessonContentGradingAndCompletion(t *testing.T) {
	ctx := context.Background()
	s, _ := gatedService(t, fakeEntitlements{"paid|lesson-numbers": true}, nil)

	for _, v := range []usecase.Viewer{{}, learner("free")} {
		lwi, err := s.GetLesson(ctx, v, "lesson-numbers")
		if err != nil {
			t.Fatal(err)
		}
		if !lwi.Locked || len(lwi.Items) != 0 {
			t.Fatalf("viewer %q: want locked lesson without items, got locked=%v items=%d", v.UserID, lwi.Locked, len(lwi.Items))
		}
	}
	if _, err := s.GradeExercise(ctx, learner("free"), "ex-numbers-1", json.RawMessage(`{"choice":0}`)); !errors.Is(err, domain.ErrLessonLocked) {
		t.Fatalf("grading a locked lesson: want ErrLessonLocked, got %v", err)
	}
	if _, err := s.CompleteLesson(ctx, learner("free"), "lesson-numbers"); !errors.Is(err, domain.ErrLessonLocked) {
		t.Fatalf("completing a locked lesson: want ErrLessonLocked, got %v", err)
	}

	lwi, err := s.GetLesson(ctx, learner("paid"), "lesson-numbers")
	if err != nil || lwi.Locked || len(lwi.Items) == 0 {
		t.Fatalf("entitled learner gets the content: locked=%v items=%d err=%v", lwi.Locked, len(lwi.Items), err)
	}
	if lwi, _ := s.GetLesson(ctx, usecase.Viewer{UserID: "admin", Admin: true}, "lesson-numbers"); lwi.Locked {
		t.Fatal("admins are never locked out")
	}

	// A free lesson is open to everyone, guests included.
	_ = s.SetAccess(ctx, "lesson", "lesson-numbers", domain.AccessFree)
	if lwi, _ := s.GetLesson(ctx, usecase.Viewer{}, "lesson-numbers"); lwi.Locked || len(lwi.Items) == 0 {
		t.Fatal("a free lesson must be open to guests")
	}
}

func TestUnpublishedCourseIsNotFound(t *testing.T) {
	ctx := context.Background()
	s, r := gatedService(t, fakeEntitlements{}, nil)
	c := &domain.Course{ID: "draft", Language: "en", CEFR: "A2", Title: "Draft", Published: false}
	_ = r.CreateCourse(ctx, c)
	u := &domain.Unit{CourseID: c.ID, Title: "U"}
	_ = r.CreateUnit(ctx, u)
	l := &domain.Lesson{UnitID: u.ID, Title: "Secret"}
	_ = r.CreateLesson(ctx, l)
	_ = r.CreateItem(ctx, &domain.LessonItem{LessonID: l.ID, Kind: domain.KindText, Payload: json.RawMessage(`{"phrase":"private"}`)})
	_ = s.SetAccess(ctx, "lesson", l.ID, domain.AccessFree)

	if _, err := s.GetCourse(ctx, usecase.Viewer{}, c.ID); !errors.Is(err, domain.ErrCourseNotFound) {
		t.Fatalf("unpublished course: want not found, got %v", err)
	}
	if _, err := s.GetLesson(ctx, learner("u1"), l.ID); !errors.Is(err, domain.ErrLessonNotFound) {
		t.Fatalf("unpublished lesson: want not found, got %v", err)
	}
	if _, err := s.LessonRequirement(ctx, l.ID); !errors.Is(err, domain.ErrLessonNotFound) {
		t.Fatalf("billing must not see an unpublished lesson, got %v", err)
	}
	if _, err := s.LessonRequirement(ctx, "no-such-lesson"); err == nil {
		t.Fatal("billing must not see a missing lesson")
	}
	if _, err := s.GetCourse(ctx, usecase.Viewer{UserID: "a", Admin: true}, c.ID); err != nil {
		t.Fatalf("admins preview unpublished courses: %v", err)
	}
}

// Completion is decided by the server: every (non-speak) exercise answered,
// and the reward is granted once however often completion is requested.
func TestCompletionCriteriaAndOneTimeReward(t *testing.T) {
	ctx := context.Background()
	rw := &onceRewarder{}
	s, _ := gatedService(t, fakeEntitlements{"u1|lesson-greetings": true}, rw)
	v := learner("u1")

	if _, err := s.CompleteLesson(ctx, v, "lesson-greetings"); !errors.Is(err, domain.ErrLessonIncomplete) {
		t.Fatalf("unanswered lesson: want ErrLessonIncomplete, got %v", err)
	}
	answerAll(t, s, v, "lesson-greetings") // the speak exercise is optional
	before := rw.xp
	for i := 0; i < 3; i++ {
		res, err := s.CompleteLesson(ctx, v, "lesson-greetings")
		if err != nil {
			t.Fatal(err)
		}
		if res.AwardedXP != (i == 0) {
			t.Fatalf("completion %d: awarded=%v", i+1, res.AwardedXP)
		}
	}
	if got := rw.xp - before; got != usecase.XPLessonComplete {
		t.Fatalf("repeated completion must award %d XP once, got %d", usecase.XPLessonComplete, got)
	}
}

// Lessons without content (imported checkpoints) are hidden from the learner's
// path, excluded from progress, and cannot be completed for rewards.
func TestEmptyLessonsAreHiddenAndNotCompletable(t *testing.T) {
	ctx := context.Background()
	rw := &onceRewarder{}
	s, r := gatedService(t, fakeEntitlements{}, rw)
	empty := &domain.Lesson{UnitID: "unit-basics", Title: "Checkpoint"}
	_ = r.CreateLesson(ctx, empty)
	_ = s.SetAccess(ctx, "lesson", empty.ID, domain.AccessFree)

	cwl, err := s.GetCourse(ctx, usecase.Viewer{}, "course-en-a1")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range cwl.Lessons {
		if l.ID == empty.ID {
			t.Fatal("an empty lesson must not be on the learner's path")
		}
	}
	if _, err := s.CompleteLesson(ctx, learner("u1"), empty.ID); !errors.Is(err, domain.ErrLessonEmpty) {
		t.Fatalf("want ErrLessonEmpty, got %v", err)
	}
	if rw.awards != 0 {
		t.Fatal("an empty lesson must not earn XP")
	}
	sum, _ := s.CourseProgress(ctx, "u1", "course-en-a1")
	if sum.TotalLessons != len(cwl.Lessons) {
		t.Fatalf("progress counts %d lessons, the path shows %d", sum.TotalLessons, len(cwl.Lessons))
	}
}

type fakeScores map[string]int

func (f fakeScores) LatestScore(_ context.Context, userID, exerciseID string) (int, bool, error) {
	n, ok := f[userID+"|"+exerciseID]
	return n, ok, nil
}

// Speaking credit comes from the server's pronunciation score; a score sent by
// the client is ignored.
func TestSpeakUsesServerScore(t *testing.T) {
	ctx := context.Background()
	s, _ := gatedService(t, fakeEntitlements{"u1|lesson-greetings": true, "u2|lesson-greetings": true}, nil)
	s.SetSpeakingScores(fakeScores{"u1|ex-g3-2": 40})

	r, err := s.GradeExercise(ctx, learner("u1"), "ex-g3-2", json.RawMessage(`{"score":999999}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Correct {
		t.Fatal("a client score must not override the server's 40")
	}
	if _, err := s.GradeExercise(ctx, learner("u2"), "ex-g3-2", json.RawMessage(`{"score":100}`)); !errors.Is(err, domain.ErrNoAssessment) {
		t.Fatalf("no recorded assessment: want ErrNoAssessment, got %v", err)
	}
}
