package usecase_test

import (
	"context"
	"encoding/json"
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

	cwl, err := s.GetCourse(ctx, cid)
	if err != nil || len(cwl.Lessons) < 2 {
		t.Fatalf("course: %v lessons=%d", err, len(cwl.Lessons))
	}
	if cwl.Lessons[0].Position > cwl.Lessons[1].Position {
		t.Fatal("lessons not ordered by position")
	}

	lwi, err := s.GetLesson(ctx, cwl.Lessons[0].ID)
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
	const user = "user-1"

	if _, err := s.GetCourse(ctx, "nope"); err == nil {
		t.Fatal("expected not found")
	}

	sum, err := s.CourseProgress(ctx, user, cid)
	if err != nil || sum.Percent != 0 {
		t.Fatalf("initial progress: %+v err=%v", sum, err)
	}

	if err := s.CompleteLesson(ctx, user, "lesson-greetings"); err != nil {
		t.Fatal(err)
	}
	// completing the same lesson twice must not double-count
	_ = s.CompleteLesson(ctx, user, "lesson-greetings")

	sum, _ = s.CourseProgress(ctx, user, cid)
	if sum.CompletedLessons != 1 || sum.TotalLessons < 2 || sum.Percent != 100*sum.CompletedLessons/sum.TotalLessons {
		t.Fatalf("after one completion: %+v", sum)
	}

	if err := s.CompleteLesson(ctx, user, "missing-lesson"); err == nil {
		t.Fatal("completing a missing lesson should error")
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
	r, err := s.GradeExercise(ctx, "u1", "ex-numbers-1", json.RawMessage(`{"choice":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Correct || r.AwardedXP || rw.awards != 0 {
		t.Fatalf("wrong first attempt must earn nothing: correct=%v awarded=%v awards=%d", r.Correct, r.AwardedXP, rw.awards)
	}

	// Same user corrects it on a later attempt → still no XP (was wrong first).
	r, err = s.GradeExercise(ctx, "u1", "ex-numbers-1", json.RawMessage(`{"choice":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Correct || r.FirstTry || r.AwardedXP || rw.awards != 0 {
		t.Fatalf("corrected-after-wrong must not earn XP: %+v awards=%d", r, rw.awards)
	}

	// A different user gets it right first try → XP awarded exactly once.
	r, err = s.GradeExercise(ctx, "u2", "ex-numbers-1", json.RawMessage(`{"choice":0}`))
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
	if _, err := s.GradeExercise(ctx, "u2", "ex-numbers-1", json.RawMessage(`{"choice":0}`)); err != nil {
		t.Fatal(err)
	}
	if rw.awards != 1 {
		t.Fatalf("re-submit must not re-award, got %d awards", rw.awards)
	}
}
