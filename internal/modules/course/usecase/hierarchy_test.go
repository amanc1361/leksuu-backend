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

// Units are created out of order and lessons interleaved, the way a re-import
// that adopts and moves things can leave positions: GetCourse must still
// return chapter 1 lessons 1..n, then chapter 2, by position — never by
// creation order and never interleaved by the per-unit lesson position.
func TestGetCourseReturnsUnitHierarchyInSourceOrder(t *testing.T) {
	ctx := context.Background()
	courses := repo.NewMemoryCourseRepo()
	svc := usecase.NewService(courses, repo.NewMemoryProgressRepo(courses.LessonCourseID), nil, time.Now)
	c := &domain.Course{Language: "en", CEFR: "A1", Title: "Imported", Published: true}
	if err := courses.CreateCourse(ctx, c); err != nil {
		t.Fatal(err)
	}
	ch2 := &domain.Unit{CourseID: c.ID, Title: "Greetings", Position: 2, SourceID: "ch2"}
	ch1 := &domain.Unit{CourseID: c.ID, Title: "Introductions", Position: 1, SourceID: "ch1"}
	for _, u := range []*domain.Unit{ch2, ch1} {
		if err := courses.CreateUnit(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	add := func(u *domain.Unit, title string, pos int) {
		if err := courses.CreateLesson(ctx, &domain.Lesson{UnitID: u.ID, Title: title, Position: pos}); err != nil {
			t.Fatal(err)
		}
	}
	add(ch2, "2.2", 2)
	add(ch1, "1.2", 2)
	add(ch2, "2.1", 1)
	add(ch1, "1.1", 1)
	add(ch1, "1.3", 3)

	got, err := svc.GetCourse(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Units) != 2 || got.Units[0].Unit.ID != ch1.ID || got.Units[1].Unit.ID != ch2.ID {
		t.Fatalf("units out of order: %+v", got.Units)
	}
	want := [][]string{{"1.1", "1.2", "1.3"}, {"2.1", "2.2"}}
	var flat []string
	for i, u := range got.Units {
		if len(u.Lessons) != len(want[i]) {
			t.Fatalf("unit %d has %d lessons", i, len(u.Lessons))
		}
		for j, l := range u.Lessons {
			if l.Title != want[i][j] || l.UnitID != u.Unit.ID {
				t.Fatalf("unit %d lesson %d = %q (unit %s)", i, j, l.Title, l.UnitID)
			}
			flat = append(flat, l.Title)
		}
	}
	for i, l := range got.Lessons {
		if l.Title != flat[i] {
			t.Fatalf("flat sequence %d = %q, want %q (must follow units, not per-unit position)", i, l.Title, flat[i])
		}
	}
}

func TestGetLessonAddsTranslationsWithoutTouchingSource(t *testing.T) {
	ctx := context.Background()
	courses := repo.NewMemoryCourseRepo()
	svc := usecase.NewService(courses, repo.NewMemoryProgressRepo(courses.LessonCourseID), nil, time.Now)
	c := &domain.Course{Language: "en", CEFR: "A1", Title: "Imported"}
	_ = courses.CreateCourse(ctx, c)
	u := &domain.Unit{CourseID: c.ID, Title: "Greetings"}
	_ = courses.CreateUnit(ctx, u)
	l := &domain.Lesson{UnitID: u.ID, Title: "Saying how you are"}
	_ = courses.CreateLesson(ctx, l)
	rule := `"How's it going?" has a similar meaning to "How are you?".`
	payload, _ := json.Marshal(map[string]any{
		"source_lang": "en", "title": "Formal and casual",
		"rule":  map[string]string{"en": rule},
		"forms": []map[string]string{{"subject": "more formal", "form": "How are you?", "audio": "a.mp3"}},
	})
	if _, err := courses.ReplaceImportedItems(ctx, l.ID, []domain.ImportedItem{{Kind: domain.KindGrammar, SourceRef: "tip", Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	_ = courses.UpsertContentTranslations(ctx, []domain.ContentTranslation{
		{SourceHash: domain.SourceHash(rule), Locale: "fa", SourceText: rule, Text: "معنی «How's it going?» شبیه «How are you?» است.", Origin: "human"},
		{SourceHash: domain.SourceHash(rule), Locale: "ckb", SourceText: rule, Text: "واتای «How's it going?» وەک «How are you?» وایە.", Origin: "human"},
		{SourceHash: domain.SourceHash("more formal"), Locale: "fa", SourceText: "more formal", Text: "رسمی‌تر", Origin: "human"},
		// A spoken example must never be replaced, even if someone stored a translation for it.
		{SourceHash: domain.SourceHash("How are you?"), Locale: "fa", SourceText: "How are you?", Text: "حالت چطور است؟", Origin: "human"},
	})

	lwi, err := svc.GetLesson(ctx, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Rule  map[string]string `json:"rule"`
		Forms []struct {
			Subject   string            `json:"subject"`
			Form      string            `json:"form"`
			SubjectTr map[string]string `json:"subject_tr"`
			FormTr    map[string]string `json:"form_tr"`
		} `json:"forms"`
	}
	if err := json.Unmarshal(lwi.Items[0].Item.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Rule["en"] != rule || p.Rule["fa"] == "" || p.Rule["ckb"] == "" {
		t.Fatalf("rule = %+v", p.Rule)
	}
	f := p.Forms[0]
	if f.Subject != "more formal" || f.SubjectTr["fa"] != "رسمی‌تر" || f.Form != "How are you?" || f.FormTr != nil {
		t.Fatalf("form = %+v (example must stay English)", f)
	}
	// The stored payload itself is unchanged: translations are applied on read.
	items, _ := courses.Items(ctx, l.ID)
	if string(items[0].Payload) != string(payload) {
		t.Fatal("source payload was modified")
	}
}
