package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/assignment/domain"
	"github.com/leksuu/platform/internal/modules/assignment/repo"
)

type fakeTutors map[string]bool

func (f fakeTutors) IsTutor(_ context.Context, id string) (bool, error) { return f[id], nil }

type fakeDir map[string]string

func (f fakeDir) DisplayNames(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := f[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

func newSvc() (*Service, context.Context) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo.NewMemoryAssignmentRepo(),
		fakeTutors{"tutor-1": true},
		fakeDir{"tutor-1": "Teacher", "stu-1": "Ali"},
		func() time.Time { return now })
	return svc, context.Background()
}

func TestAssignmentFullFlow(t *testing.T) {
	svc, ctx := newSvc()

	// Non-tutor cannot assign.
	if _, err := svc.Create(ctx, "stu-1", CreateInput{StudentID: "stu-2", Type: "task", Title: "x", Prompt: "y"}); err == nil {
		t.Fatal("non-tutor must not assign")
	}

	// Tutor creates a task.
	a, err := svc.Create(ctx, "tutor-1", CreateInput{StudentID: "stu-1", Type: "task", Title: "Past tense", Prompt: "Write 5 sentences."})
	if err != nil {
		t.Fatal(err)
	}
	if a.Status() != domain.StatusAssigned || a.Score != -1 {
		t.Fatalf("bad initial: %+v", a)
	}

	// Validation: task without prompt fails.
	if _, err := svc.Create(ctx, "tutor-1", CreateInput{StudentID: "stu-1", Type: "task", Title: "t"}); err == nil {
		t.Fatal("task needs a prompt")
	}

	// Cannot grade before submission.
	if _, err := svc.Grade(ctx, "tutor-1", a.ID, 90, "good"); err == nil {
		t.Fatal("cannot grade unsubmitted")
	}

	// Wrong student cannot submit.
	if _, err := svc.Submit(ctx, "stu-2", a.ID, "answer"); err == nil {
		t.Fatal("wrong student must not submit")
	}

	// Student submits.
	sub, err := svc.Submit(ctx, "stu-1", a.ID, "I went. She ate. ...")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status() != domain.StatusSubmitted {
		t.Fatalf("want submitted, got %s", sub.Status())
	}

	// Bad score rejected.
	if _, err := svc.Grade(ctx, "tutor-1", a.ID, 150, "x"); err == nil {
		t.Fatal("score>100 must fail")
	}
	// Wrong tutor cannot grade.
	if _, err := svc.Grade(ctx, "tutor-2", a.ID, 80, "x"); err == nil {
		t.Fatal("non-owner tutor must not grade")
	}

	// Tutor grades.
	g, err := svc.Grade(ctx, "tutor-1", a.ID, 85, "Nice work, watch the verbs.")
	if err != nil {
		t.Fatal(err)
	}
	if g.Status() != domain.StatusGraded || g.Score != 85 {
		t.Fatalf("bad grade: %+v", g)
	}

	// Cannot resubmit after grading.
	if _, err := svc.Submit(ctx, "stu-1", a.ID, "again"); err == nil {
		t.Fatal("cannot resubmit graded work")
	}

	// Lists carry resolved names + status.
	tv, _ := svc.ListForTutor(ctx, "tutor-1")
	if len(tv) != 1 || tv[0].StudentName != "Ali" || tv[0].Status != domain.StatusGraded {
		t.Fatalf("tutor list = %+v", tv)
	}
	sv, _ := svc.ListForStudent(ctx, "stu-1")
	if len(sv) != 1 || sv[0].TutorName != "Teacher" {
		t.Fatalf("student list = %+v", sv)
	}
}
