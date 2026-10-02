package usecase

import (
	"context"
	"testing"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

func TestAddLessonToUnitReusesImportedSourceID(t *testing.T) {
	ctx := context.Background()
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)

	course := &coursedomain.Course{Language: "en", CEFR: "A1", Title: "Imported"}
	if err := repo.CreateCourse(ctx, course); err != nil {
		t.Fatal(err)
	}
	unit := &coursedomain.Unit{CourseID: course.ID, Title: "Basics"}
	if err := repo.CreateUnit(ctx, unit); err != nil {
		t.Fatal(err)
	}

	first, reused, err := svc.AddLessonToUnit(ctx, unit.ID, "Hello", "source-42", 0)
	if err != nil || reused {
		t.Fatalf("first import: reused=%v err=%v", reused, err)
	}
	second, reused, err := svc.AddLessonToUnit(ctx, unit.ID, "Hello again", "source-42", 0)
	if err != nil || !reused {
		t.Fatalf("repeat import: reused=%v err=%v", reused, err)
	}
	if first.ID != second.ID {
		t.Fatalf("repeat created a duplicate lesson: %s != %s", first.ID, second.ID)
	}
	lessons, err := repo.LessonsByUnit(ctx, unit.ID)
	if err != nil || len(lessons) != 1 {
		t.Fatalf("lessons=%d err=%v", len(lessons), err)
	}
}

func TestMoveLessonToUnit(t *testing.T) {
	ctx := context.Background()
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)

	course := &coursedomain.Course{Language: "en", CEFR: "A1", Title: "Imported"}
	other := &coursedomain.Course{Language: "en", CEFR: "A2", Title: "Other"}
	for _, c := range []*coursedomain.Course{course, other} {
		if err := repo.CreateCourse(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	from := &coursedomain.Unit{CourseID: course.ID, Title: "People and family"}
	to := &coursedomain.Unit{CourseID: course.ID, Title: "Unit 1"}
	foreign := &coursedomain.Unit{CourseID: other.ID, Title: "Unit 1"}
	for _, u := range []*coursedomain.Unit{from, to, foreign} {
		if err := repo.CreateUnit(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	l, _, err := svc.AddLessonToUnit(ctx, from.ID, "Referring to a person", "objective_1", 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.MoveLessonToUnit(ctx, l.ID, foreign.ID); err == nil {
		t.Fatal("moved a lesson into another course's unit")
	}
	if err := svc.MoveLessonToUnit(ctx, l.ID, to.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.LessonsByUnit(ctx, from.ID); len(got) != 0 {
		t.Fatalf("old unit still has %d lessons", len(got))
	}
	got, _ := repo.LessonsByUnit(ctx, to.ID)
	if len(got) != 1 || got[0].ID != l.ID {
		t.Fatalf("new unit lessons = %+v", got)
	}
}

func TestReplaceImportedItemsKeepsHandAuthoredItems(t *testing.T) {
	ctx := context.Background()
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)

	course := &coursedomain.Course{Language: "en", CEFR: "A1", Title: "Imported"}
	if err := repo.CreateCourse(ctx, course); err != nil {
		t.Fatal(err)
	}
	unit := &coursedomain.Unit{CourseID: course.ID, Title: "Basics"}
	if err := repo.CreateUnit(ctx, unit); err != nil {
		t.Fatal(err)
	}
	lesson, _, err := svc.AddLessonToUnit(ctx, unit.ID, "Hello", "source-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddItem(ctx, lesson.ID, "text", []byte(`{"phrase":"mine"}`), 0, ""); err != nil {
		t.Fatal(err)
	}
	batch := func(n int) []coursedomain.ImportedItem {
		out := make([]coursedomain.ImportedItem, n)
		for i := range out {
			out[i] = coursedomain.ImportedItem{Kind: "exercise", SourceRef: "node", Exercises: []coursedomain.Exercise{
				{Type: "mcq", Prompt: []byte(`{}`), AnswerKey: []byte(`{"correct":0}`), Difficulty: 1},
			}}
		}
		return out
	}
	if _, err := svc.ReplaceImportedItems(ctx, lesson.ID, batch(3)); err != nil {
		t.Fatal(err)
	}
	saved, err := svc.ReplaceImportedItems(ctx, lesson.ID, batch(2))
	if err != nil {
		t.Fatal(err)
	}
	items, _ := repo.Items(ctx, lesson.ID)
	if len(items) != 3 || items[0].SourceRef != "" {
		t.Fatalf("want the hand-authored item plus 2 imported, got %d items", len(items))
	}
	if saved[0].Position != 2 || saved[1].Position != 3 {
		t.Fatalf("imported items should follow hand-authored ones: %d, %d", saved[0].Position, saved[1].Position)
	}
	if exs, _ := repo.Exercises(ctx, saved[1].ID); len(exs) != 1 {
		t.Fatalf("exercise not stored with its item: %d", len(exs))
	}
	if _, err := svc.ReplaceImportedItems(ctx, lesson.ID, []coursedomain.ImportedItem{{Kind: "text"}}); err == nil {
		t.Fatal("an item without source_ref must be rejected")
	}
}
