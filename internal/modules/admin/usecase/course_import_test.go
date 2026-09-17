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
