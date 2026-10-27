package usecase

import (
	"context"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

func TestAddLessonToUnitReusesExistingSourceID(t *testing.T) {
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	ctx := context.Background()

	before, err := repo.LessonsByUnit(ctx, "unit-basics")
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := svc.AddLessonToUnit(ctx, "unit-basics", "Hello!", "objective_hello", 0)
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	second, placement, err := svc.AddLessonToUnit(ctx, "unit-basics", "Hello again", "objective_hello", 0)
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	after, err := repo.LessonsByUnit(ctx, "unit-basics")
	if err != nil {
		t.Fatal(err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same lesson id, got first=%s second=%s", first.ID, second.ID)
	}
	if !placement.Reused {
		t.Fatal("expected reused flag on duplicate source id")
	}
	if len(after) != len(before)+1 {
		t.Fatalf("expected one new lesson, before=%d after=%d", len(before), len(after))
	}
}
