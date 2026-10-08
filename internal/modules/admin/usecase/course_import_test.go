package usecase

import (
	"context"
	"errors"
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

	first, placed, err := svc.AddLessonToUnit(ctx, unit.ID, "Hello", "source-42", 0)
	if err != nil || placed.Reused {
		t.Fatalf("first import: reused=%v err=%v", placed.Reused, err)
	}
	second, placed, err := svc.AddLessonToUnit(ctx, unit.ID, "Hello again", "source-42", 0)
	if err != nil || !placed.Reused || placed.Moved {
		t.Fatalf("repeat import: placed=%+v err=%v", placed, err)
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
	res, err := svc.ReplaceImportedItems(ctx, lesson.ID, batch(2))
	if err != nil {
		t.Fatal(err)
	}
	saved := res.Items
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

// importFixture is an empty course with two units, as a first import leaves it.
func importFixture(t *testing.T) (context.Context, *courserepo.MemoryCourseRepo, *Service, *coursedomain.Course) {
	t.Helper()
	ctx := context.Background()
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	course := &coursedomain.Course{Language: "en", CEFR: "A1", Title: "Imported"}
	if err := repo.CreateCourse(ctx, course); err != nil {
		t.Fatal(err)
	}
	return ctx, repo, svc, course
}

func TestAddLessonToUnitMovesLessonFromOtherUnitInsteadOfDuplicating(t *testing.T) {
	ctx, repo, svc, course := importFixture(t)
	wrong, _, err := svc.CreateUnit(ctx, course.ID, "People and family", "", "")
	if err != nil {
		t.Fatal(err)
	}
	right, _, err := svc.CreateUnit(ctx, course.ID, "Introductions", "", "str_chapter_1")
	if err != nil {
		t.Fatal(err)
	}
	old, _, err := svc.AddLessonToUnit(ctx, wrong.ID, "Hello!", "objective_1", 0)
	if err != nil {
		t.Fatal(err)
	}

	got, placed, err := svc.AddLessonToUnit(ctx, right.ID, "Hello!", "objective_1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !placed.Reused || !placed.Moved || got.ID != old.ID || got.UnitID != right.ID {
		t.Fatalf("lesson not moved: placed=%+v got=%+v", placed, got)
	}
	all, _ := repo.Lessons(ctx, course.ID)
	if len(all) != 1 {
		t.Fatalf("re-import duplicated the lesson: %d lessons", len(all))
	}
	// A third import is a no-op.
	if _, placed, _ := svc.AddLessonToUnit(ctx, right.ID, "Hello!", "objective_1", 0); !placed.Reused || placed.Moved {
		t.Fatalf("third import: %+v", placed)
	}
}

func TestAddLessonToUnitReportsExistingDuplicates(t *testing.T) {
	ctx, repo, svc, course := importFixture(t)
	a := &coursedomain.Unit{CourseID: course.ID, Title: "A"}
	b := &coursedomain.Unit{CourseID: course.ID, Title: "B"}
	for _, u := range []*coursedomain.Unit{a, b} {
		if err := repo.CreateUnit(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	// What an old per-unit importer could leave behind: one source, two lessons.
	for _, u := range []*coursedomain.Unit{a, b} {
		if err := repo.CreateLesson(ctx, &coursedomain.Lesson{UnitID: u.ID, Title: "Hello", SourceID: "objective_1"}); err != nil {
			t.Fatal(err)
		}
	}
	got, placed, err := svc.AddLessonToUnit(ctx, b.ID, "Hello", "objective_1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.UnitID != b.ID || placed.Moved || len(placed.Duplicates) != 1 {
		t.Fatalf("should reuse the lesson already in the unit and report the other: %+v", placed)
	}
	if all, _ := repo.Lessons(ctx, course.ID); len(all) != 2 {
		t.Fatalf("duplicates must be reported, not deleted or added: %d", len(all))
	}
}

func TestCreateUnitIsIdempotentBySourceIDAndAdoptsLegacyTitle(t *testing.T) {
	ctx, repo, svc, course := importFixture(t)
	legacy, _, err := svc.CreateUnit(ctx, course.ID, "Introductions", "", "")
	if err != nil {
		t.Fatal(err)
	}
	adopted, reused, err := svc.CreateUnit(ctx, course.ID, "  introductions ", "", "str_ch1")
	if err != nil || !reused || adopted.ID != legacy.ID || adopted.SourceID != "str_ch1" {
		t.Fatalf("legacy unit not adopted: reused=%v unit=%+v err=%v", reused, adopted, err)
	}
	again, reused, err := svc.CreateUnit(ctx, course.ID, "Introductions (renamed in source)", "", "str_ch1")
	if err != nil || !reused || again.ID != legacy.ID {
		t.Fatalf("source id should win over title: reused=%v unit=%+v", reused, again)
	}
	other, reused, err := svc.CreateUnit(ctx, course.ID, "Greetings", "", "str_ch2")
	if err != nil || reused || other.Position != 2 {
		t.Fatalf("new chapter: reused=%v unit=%+v", reused, other)
	}
	if units, _ := repo.Units(ctx, course.ID); len(units) != 2 {
		t.Fatalf("units = %d, want 2", len(units))
	}
}

func TestCreateCourseIsIdempotentBySourceID(t *testing.T) {
	ctx := context.Background()
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	in := CreateCourseInput{Language: "en", CEFR: "A1", Title: "English A1", SourceID: "busuu:course_pack_en_complete:pack_level_en_a1"}
	first, reused, err := svc.CreateCourse(ctx, in)
	if err != nil || reused {
		t.Fatalf("first: reused=%v err=%v", reused, err)
	}
	second, reused, err := svc.CreateCourse(ctx, in)
	if err != nil || !reused || second.ID != first.ID {
		t.Fatalf("retry created another course: reused=%v err=%v", reused, err)
	}
	if _, reused, _ := svc.CreateCourse(ctx, CreateCourseInput{Language: "en", Title: "Hand-made"}); reused {
		t.Fatal("a course without source id must never be reused")
	}
}

func importedItem(ref, kind, payload string, exercise string) coursedomain.ImportedItem {
	it := coursedomain.ImportedItem{Kind: kind, SourceRef: ref, Payload: []byte(payload)}
	if exercise != "" {
		it.Exercises = []coursedomain.Exercise{{Type: exercise, Prompt: []byte(payload), AnswerKey: []byte(`{"correct":0}`), Difficulty: 1}}
	}
	return it
}

func TestReplaceImportedItemsIsIdempotentAndKeepsIDs(t *testing.T) {
	ctx, repo, svc, course := importFixture(t)
	unit, _, _ := svc.CreateUnit(ctx, course.ID, "Introductions", "", "ch1")
	lesson, _, err := svc.AddLessonToUnit(ctx, unit.ID, "Hello", "objective_1", 0)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := svc.AddItem(ctx, lesson.ID, "text", []byte(`{"phrase":"mine"}`), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	source := []coursedomain.ImportedItem{
		importedItem("node_a", "text", `{"phrase":"Hello"}`, ""),
		importedItem("node_b", "exercise", `{"question":"Q1","options":["a","b"]}`, "mcq"),
		// Busuu repeats some exercise nodes inside one lesson: each is an item.
		importedItem("node_b", "exercise", `{"question":"Q1","options":["a","b"]}`, "mcq"),
		importedItem("node_c", "grammar", `{"rule":{"en":"Tip"}}`, ""),
	}
	first, err := svc.ReplaceImportedItems(ctx, lesson.ID, source)
	if err != nil || first.Created != 4 {
		t.Fatalf("first: %+v err=%v", first, err)
	}
	exBefore, _ := repo.Exercises(ctx, first.Items[1].ID)

	second, err := svc.ReplaceImportedItems(ctx, lesson.ID, source)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Updated != 0 || second.Unchanged != 4 || second.Removed != 0 {
		t.Fatalf("identical re-import changed data: %+v", second)
	}
	for i := range source {
		if first.Items[i].ID != second.Items[i].ID {
			t.Fatalf("item %d got a new ID on re-import", i)
		}
	}
	exAfter, _ := repo.Exercises(ctx, second.Items[1].ID)
	if len(exAfter) != 1 || exAfter[0].ID != exBefore[0].ID {
		t.Fatal("exercise ID changed on identical re-import (learner attempts would be orphaned)")
	}

	// The source changes: node_b's question is edited, node_c disappears,
	// node_d is new. Manual content stays first and untouched.
	changed := []coursedomain.ImportedItem{
		source[0],
		importedItem("node_b", "exercise", `{"question":"Q1 (fixed)","options":["a","b"]}`, "mcq"),
		source[2],
		importedItem("node_d", "exercise", `{"question":"Q2","options":["c","d"]}`, "mcq"),
	}
	third, err := svc.ReplaceImportedItems(ctx, lesson.ID, changed)
	if err != nil {
		t.Fatal(err)
	}
	if third.Created != 1 || third.Updated != 1 || third.Unchanged != 2 || third.Removed != 1 {
		t.Fatalf("changed source: %+v", third)
	}
	if third.Items[1].ID != first.Items[1].ID {
		t.Fatal("edited item should be updated in place")
	}
	items, _ := repo.Items(ctx, lesson.ID)
	if len(items) != 5 || items[0].ID != manual.ID {
		t.Fatalf("want manual item first plus 4 imported, got %d", len(items))
	}
	for i, it := range items {
		if it.Position != i+1 {
			t.Fatalf("positions not contiguous: item %d at %d", i, it.Position)
		}
	}
	if exs, _ := repo.Exercises(ctx, third.Items[1].ID); len(exs) != 1 || string(exs[0].Prompt) != `{"question":"Q1 (fixed)","options":["a","b"]}` {
		t.Fatalf("exercise not updated: %+v", exs)
	}
}

func TestCourseTranslationsKeepSourceAndSurviveReimport(t *testing.T) {
	ctx, _, svc, course := importFixture(t)
	unit, _, _ := svc.CreateUnit(ctx, course.ID, "Greetings", "", "ch2")
	lesson, _, _ := svc.AddLessonToUnit(ctx, unit.ID, "Saying how you are", "objective_2", 0)
	tip := importedItem("tip_1", "grammar", `{"source_lang":"en","rule":{"en":"\"How's it going?\" has a similar meaning to \"How are you?\"."},`+
		`"forms":[{"subject":"more formal","form":"How are you?","audio":"a.mp3"},{"subject":"more casual","form":"How's it going?","audio":"b.mp3"}]}`, "")
	if _, err := svc.ReplaceImportedItems(ctx, lesson.ID, []coursedomain.ImportedItem{tip}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.CourseTranslations(ctx, course.ID, "fa")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, s := range list {
		kinds[s.Source] = s.Kind
	}
	if len(list) != 3 || kinds["more formal"] != coursedomain.L10nTable || kinds["How are you?"] != "" {
		t.Fatalf("spoken examples must not be listed for translation: %+v", list)
	}
	if _, _, err := svc.SaveTranslations(ctx, "fa", []TranslationInput{{Source: list[0].Source, Text: "«How's it going?» معنایی شبیه «How are you?» دارد."}}); err != nil {
		t.Fatal(err)
	}
	// Re-import the same tip: the translation is keyed by the source text, so it stays.
	if _, err := svc.ReplaceImportedItems(ctx, lesson.ID, []coursedomain.ImportedItem{tip}); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.CourseTranslations(ctx, course.ID, "fa")
	if list[0].Translation == "" || list[0].Origin != coursedomain.OriginHuman {
		t.Fatalf("translation lost on re-import: %+v", list[0])
	}
	if ckb, _ := svc.CourseTranslations(ctx, course.ID, "ckb"); ckb[0].Translation != "" {
		t.Fatal("fa translation leaked into ckb")
	}
	if _, _, err := svc.SaveTranslations(ctx, "xx", []TranslationInput{{Source: "a", Text: "b"}}); err == nil {
		t.Fatal("unknown locale accepted")
	}
	// Empty text deletes.
	if _, deleted, _ := svc.SaveTranslations(ctx, "fa", []TranslationInput{{Source: list[0].Source}}); deleted != 1 {
		t.Fatal("empty translation should delete")
	}
}

// A legacy database can hold the real lesson (with items and learner data) in
// a renamed unit and an empty copy elsewhere. Re-import must keep and move the
// real lesson, never fill the empty shell and orphan the real one.
func TestAddLessonToUnitKeepsTheCopyWithContent(t *testing.T) {
	ctx, repo, svc, course := importFixture(t)
	legacy := &coursedomain.Unit{CourseID: course.ID, Title: "Unit 1"}
	other := &coursedomain.Unit{CourseID: course.ID, Title: "Other"}
	target := &coursedomain.Unit{CourseID: course.ID, Title: "Introductions"}
	for _, u := range []*coursedomain.Unit{other, legacy, target} {
		if err := repo.CreateUnit(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	shell := &coursedomain.Lesson{UnitID: other.ID, Title: "Hello! (old copy)", SourceID: "objective_1"}
	real := &coursedomain.Lesson{UnitID: legacy.ID, Title: "Hello!", SourceID: "objective_1"}
	for _, l := range []*coursedomain.Lesson{shell, real} {
		if err := repo.CreateLesson(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateItem(ctx, &coursedomain.LessonItem{LessonID: real.ID, Kind: "text", Payload: []byte(`{"phrase":"teacher note"}`)}); err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 2; run++ {
		got, placed, err := svc.AddLessonToUnit(ctx, target.ID, "Hello!", "objective_1", 0)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != real.ID || got.UnitID != target.ID || len(placed.Duplicates) != 1 || placed.Duplicates[0] != shell.ID {
			t.Fatalf("run %d: picked %s (real %s), placed %+v", run, got.ID, real.ID, placed)
		}
	}
}

// A unit holds one lesson per source: moving a second copy into a unit that
// already has one is a conflict (409), never an internal error or a duplicate.
func TestMoveLessonIntoUnitWithSameSourceIsConflict(t *testing.T) {
	ctx, repo, _, course := importFixture(t)
	a := &coursedomain.Unit{CourseID: course.ID, Title: "A"}
	b := &coursedomain.Unit{CourseID: course.ID, Title: "B"}
	for _, u := range []*coursedomain.Unit{a, b} {
		if err := repo.CreateUnit(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	inA := &coursedomain.Lesson{UnitID: a.ID, Title: "Hello", SourceID: "objective_1"}
	inB := &coursedomain.Lesson{UnitID: b.ID, Title: "Hello", SourceID: "objective_1"}
	for _, l := range []*coursedomain.Lesson{inA, inB} {
		if err := repo.CreateLesson(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.MoveLessonToUnit(ctx, inB.ID, a.ID); !errors.Is(err, coursedomain.ErrLessonSourceConflict) {
		t.Fatalf("err = %v, want ErrLessonSourceConflict", err)
	}
}
