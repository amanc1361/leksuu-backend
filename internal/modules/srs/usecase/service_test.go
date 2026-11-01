package usecase

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/modules/srs/repo"
)

var schedClock = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestRescheduleGoodProgression(t *testing.T) {
	// A fresh card graduated with successive "good": 1 -> 6 -> round(6*ease).
	in := domain.SchedInput{IntervalDays: 0, Reps: 0, Ease: domain.EaseDefault}
	r1 := domain.Reschedule(in, domain.GradeGood, schedClock)
	if r1.IntervalDays != 1 || r1.Reps != 1 {
		t.Fatalf("first good: iv=%d reps=%d, want 1/1", r1.IntervalDays, r1.Reps)
	}
	r2 := domain.Reschedule(domain.SchedInput{IntervalDays: r1.IntervalDays, Reps: r1.Reps, Ease: r1.Ease}, domain.GradeGood, schedClock)
	if r2.IntervalDays != 6 || r2.Reps != 2 {
		t.Fatalf("second good: iv=%d reps=%d, want 6/2", r2.IntervalDays, r2.Reps)
	}
	r3 := domain.Reschedule(domain.SchedInput{IntervalDays: r2.IntervalDays, Reps: r2.Reps, Ease: r2.Ease}, domain.GradeGood, schedClock)
	want := 15 // round(6 * 2.5)
	if r3.IntervalDays != want {
		t.Fatalf("third good: iv=%d, want %d", r3.IntervalDays, want)
	}
	if !r3.Due.Equal(schedClock.AddDate(0, 0, want)) {
		t.Fatalf("third good due=%v", r3.Due)
	}
}

func TestRescheduleAgainLapses(t *testing.T) {
	in := domain.SchedInput{IntervalDays: 30, Reps: 5, Lapses: 1, Ease: 2.5}
	r := domain.Reschedule(in, domain.GradeAgain, schedClock)
	if r.IntervalDays != 0 {
		t.Fatalf("again interval=%d, want 0", r.IntervalDays)
	}
	if r.Reps != 0 {
		t.Fatalf("again reps=%d, want reset to 0", r.Reps)
	}
	if r.Lapses != 2 {
		t.Fatalf("again lapses=%d, want 2", r.Lapses)
	}
	if r.Ease >= 2.5 {
		t.Fatalf("again ease=%.2f, want lower than 2.5", r.Ease)
	}
	if !r.Due.After(schedClock) || r.Due.After(schedClock.Add(time.Hour)) {
		t.Fatalf("again due=%v, want within the hour", r.Due)
	}
}

func TestRescheduleHardAndEasyModifiers(t *testing.T) {
	base := domain.SchedInput{IntervalDays: 10, Reps: 3, Ease: 2.5}
	hard := domain.Reschedule(base, domain.GradeHard, schedClock)
	if hard.IntervalDays != 12 { // round(10*1.2)
		t.Fatalf("hard iv=%d, want 12", hard.IntervalDays)
	}
	if hard.Ease >= 2.5 {
		t.Fatalf("hard should lower ease, got %.2f", hard.Ease)
	}
	easy := domain.Reschedule(base, domain.GradeEasy, schedClock)
	good := domain.Reschedule(base, domain.GradeGood, schedClock)
	if !(easy.IntervalDays > good.IntervalDays && good.IntervalDays > hard.IntervalDays) {
		t.Fatalf("ordering broken: hard=%d good=%d easy=%d", hard.IntervalDays, good.IntervalDays, easy.IntervalDays)
	}
	if easy.Ease <= 2.5 {
		t.Fatalf("easy should raise ease, got %.2f", easy.Ease)
	}
}

func TestEaseFloorAndCap(t *testing.T) {
	// Repeated "again" must never drop ease below the floor.
	e := domain.EaseDefault
	for i := 0; i < 20; i++ {
		e = domain.Reschedule(domain.SchedInput{IntervalDays: 5, Ease: e}, domain.GradeAgain, schedClock).Ease
	}
	if e < domain.EaseMin-1e-9 {
		t.Fatalf("ease floor breached: %.3f < %.3f", e, domain.EaseMin)
	}
	// Repeated "easy" must never exceed the cap.
	e = domain.EaseDefault
	for i := 0; i < 20; i++ {
		e = domain.Reschedule(domain.SchedInput{IntervalDays: 5, Ease: e}, domain.GradeEasy, schedClock).Ease
	}
	if e > domain.EaseMax+1e-9 {
		t.Fatalf("ease cap breached: %.3f > %.3f", e, domain.EaseMax)
	}
}

func TestMaxIntervalCap(t *testing.T) {
	r := domain.Reschedule(domain.SchedInput{IntervalDays: 10000, Reps: 9, Ease: 3.0}, domain.GradeEasy, schedClock)
	if r.IntervalDays > domain.MaxInterval {
		t.Fatalf("interval %d exceeds cap %d", r.IntervalDays, domain.MaxInterval)
	}
}

func TestPreviewIntervals(t *testing.T) {
	in := domain.SchedInput{IntervalDays: 10, Reps: 3, Ease: 2.5}
	p := domain.PreviewIntervals(in, schedClock)
	for _, g := range []string{"again", "hard", "good", "easy"} {
		if _, ok := p[g]; !ok {
			t.Fatalf("preview missing grade %q", g)
		}
	}
	if p["again"] != 0 {
		t.Fatalf("again preview=%d, want 0", p["again"])
	}
	if !(p["easy"] >= p["good"] && p["good"] >= p["hard"]) {
		t.Fatalf("preview ordering broken: %+v", p)
	}
}

func TestZeroEaseDefaults(t *testing.T) {
	// A card with an uninitialized ease should behave as if ease=default.
	r := domain.Reschedule(domain.SchedInput{IntervalDays: 6, Reps: 2, Ease: 0}, domain.GradeGood, schedClock)
	want := 15 // round(6 * 2.5)
	if math.Abs(float64(r.IntervalDays-want)) > 0 {
		t.Fatalf("zero-ease good iv=%d, want %d", r.IntervalDays, want)
	}
}

func TestUndoRestoresScheduleOnce(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemorySRSRepo()
	svc := NewService(r, nil, func() time.Time { return now })
	ctx := context.Background()

	c, err := svc.AddCard(ctx, "u1", AddInput{Language: "en", Lemma: "ephemeral", Meaning: map[string]string{"fa": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := r.Card(ctx, "u1", c.ID)

	if _, err := svc.Review(ctx, "u1", c.ID, domain.GradeGood); err != nil {
		t.Fatal(err)
	}
	mid, _ := r.Card(ctx, "u1", c.ID)
	if mid.DueAt.Equal(before.DueAt) {
		t.Fatal("review should move the due date")
	}

	got, err := svc.Undo(ctx, "u1", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DueAt.Equal(before.DueAt) || got.Reps != before.Reps || got.IntervalDays != before.IntervalDays || got.Ease != before.Ease {
		t.Fatalf("undo mismatch: got %+v want %+v", got, before)
	}
	// One level only.
	if _, err := svc.Undo(ctx, "u1", c.ID); err == nil {
		t.Fatal("second undo must fail")
	}
	// Ownership.
	if _, err := svc.Undo(ctx, "intruder", c.ID); err == nil {
		t.Fatal("foreign undo must fail")
	}
}

func TestWordManagerListSuspendDelete(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemorySRSRepo()
	svc := NewService(r, nil, func() time.Time { return now })
	ctx := context.Background()

	a, _ := svc.AddCard(ctx, "u1", AddInput{Language: "en", Lemma: "apple", Meaning: map[string]string{"fa": "سیب"}})
	b, _ := svc.AddCard(ctx, "u1", AddInput{Language: "en", Lemma: "banana", Meaning: map[string]string{"fa": "موز"}})
	_, _ = svc.AddCard(ctx, "intruder", AddInput{Language: "en", Lemma: "apricot", Meaning: map[string]string{"fa": "x"}})

	all, err := svc.MyCards(ctx, "u1", "", "", 50, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("all=%d err=%v, want 2 (ownership)", len(all), err)
	}
	got, _ := svc.MyCards(ctx, "u1", "app", "", 50, 0)
	if len(got) != 1 || got[0].Lemma != "apple" {
		t.Fatalf("query filter wrong: %+v", got)
	}

	// Manual suspend preserves the schedule; the card leaves "active".
	if _, err := svc.Suspend(ctx, "u1", a.ID, true); err != nil {
		t.Fatal(err)
	}
	act, _ := svc.MyCards(ctx, "u1", "", "active", 50, 0)
	sus, _ := svc.MyCards(ctx, "u1", "", "suspended", 50, 0)
	if len(act) != 1 || len(sus) != 1 || sus[0].CardID != a.ID {
		t.Fatalf("status filters wrong: act=%d sus=%d", len(act), len(sus))
	}
	// Suspended cards never show up as due.
	due, _ := svc.Due(ctx, "u1", "en", 10)
	for _, d := range due {
		if d.CardID == a.ID {
			t.Fatal("suspended card must not be due")
		}
	}
	// Resume keeps the schedule (still due now, box unchanged).
	if _, err := svc.Suspend(ctx, "u1", a.ID, false); err != nil {
		t.Fatal(err)
	}

	// Delete: ownership enforced; gone afterwards.
	if err := svc.DeleteCard(ctx, "intruder", b.ID); err == nil {
		t.Fatal("foreign delete must fail")
	}
	if err := svc.DeleteCard(ctx, "u1", b.ID); err != nil {
		t.Fatal(err)
	}
	left, _ := svc.MyCards(ctx, "u1", "", "", 50, 0)
	if len(left) != 1 {
		t.Fatalf("after delete: %d, want 1", len(left))
	}
}

// QA N04: from every early/mature state, Easy must never be shorter than Good,
// and each preview label must equal the interval actually saved (due date).
func TestEasyNeverShorterThanGoodAndPreviewMatchesSaved(t *testing.T) {
	states := []domain.SchedInput{
		{IntervalDays: 0, Reps: 0, Ease: 2.5},  // new
		{IntervalDays: 1, Reps: 1, Ease: 2.5},  // box 2 — the reported case (Good 6d, Easy 3d)
		{IntervalDays: 6, Reps: 2, Ease: 2.5},  // second step
		{IntervalDays: 1, Reps: 1, Ease: 1.3},  // floor ease
		{IntervalDays: 10, Reps: 3, Ease: 2.5}, // mature
	}
	for _, in := range states {
		p := domain.PreviewIntervals(in, schedClock)
		if p["easy"] < p["good"] {
			t.Fatalf("state %+v: easy %dd shorter than good %dd", in, p["easy"], p["good"])
		}
		for _, g := range []domain.Grade{domain.GradeHard, domain.GradeGood, domain.GradeEasy} {
			res := domain.Reschedule(in, g, schedClock)
			days := int(math.Round(res.Due.Sub(schedClock).Hours() / 24))
			if days != p[string(g)] {
				t.Fatalf("state %+v grade %s: label %dd but saved due is %dd", in, g, p[string(g)], days)
			}
		}
	}
}

// QA N09: lookup folds case and sentence punctuation and finds phrases too.
func TestLookupNormalizesCaseAndPunctuation(t *testing.T) {
	cases := map[string]string{"Hello!": "hello", " Hi! ": "hi", "Meet": "meet", "are": "are", "I'm": "i'm", "¿Qué?": "qué"}
	for in, want := range cases {
		if got := domain.NormalizeLemma(in); got != want {
			t.Fatalf("NormalizeLemma(%q)=%q want %q", in, got, want)
		}
	}
	r := repo.NewMemorySRSRepo()
	svc := NewService(r, nil, func() time.Time { return schedClock })
	ctx := context.Background()
	_ = r.EnsureVocab(ctx, &domain.Vocab{Language: "en", Kind: "word", Lemma: "meet", Meaning: map[string]string{"fa": "ملاقات کردن"}})
	_ = r.EnsureVocab(ctx, &domain.Vocab{Language: "en", Kind: "phrase", Lemma: "Hello!", Meaning: map[string]string{}})
	_ = r.EnsureVocab(ctx, &domain.Vocab{Language: "en", Kind: "word", Lemma: "hello", Meaning: map[string]string{"fa": "سلام"}})
	for _, q := range []string{"Meet", "meet", "Hello!", "hello", "HELLO"} {
		v, err := svc.Lookup(ctx, "en", q)
		if err != nil {
			t.Fatalf("lookup %q: %v", q, err)
		}
		if v.Meaning["fa"] == "" {
			t.Fatalf("lookup %q returned an entry without a meaning: %+v", q, v)
		}
	}
	if _, err := svc.Lookup(ctx, "en", "!!!"); err == nil {
		t.Fatal("punctuation-only lookup should not match")
	}
}

func TestLookupUsesExistingEnglishInflectionsAndLearnerMeanings(t *testing.T) {
	r := repo.NewMemorySRSRepo()
	svc := NewService(r, nil, func() time.Time { return schedClock })
	ctx := context.Background()
	_ = r.EnsureVocab(ctx, &domain.Vocab{
		Language: "en", Kind: "word", Lemma: "and",
		Meaning: map[string]string{"ckb": "و", "fa": "و"},
	})
	_ = r.EnsureVocab(ctx, &domain.Vocab{
		Language: "en", Kind: "word", Lemma: "celebrate",
		Meaning:       map[string]string{"ckb": "جەژن گێڕان", "fa": "جشن گرفتن"},
		Pronunciation: "/ˈselɪbreɪt/",
	})

	common, err := svc.Lookup(ctx, "en", "And,")
	if err != nil || common.Meaning["ckb"] == "" || common.Meaning["fa"] == "" {
		t.Fatalf("common English word should expose both learner meanings: v=%+v err=%v", common, err)
	}
	inflected, err := svc.Lookup(ctx, "en", "Celebrated")
	if err != nil || inflected.Lemma != "celebrate" || inflected.Pronunciation == "" {
		t.Fatalf("inflected lookup should resolve an existing base entry: v=%+v err=%v", inflected, err)
	}
}

func TestLookupDoesNotInventMissingWordsOrProperNouns(t *testing.T) {
	r := repo.NewMemorySRSRepo()
	svc := NewService(r, nil, func() time.Time { return schedClock })
	ctx := context.Background()
	_ = r.EnsureVocab(ctx, &domain.Vocab{Language: "en", Kind: "word", Lemma: "simple", Meaning: map[string]string{"ckb": "سادە"}})

	for _, word := range []string{"unlisted", "Simon"} {
		if v, err := svc.Lookup(ctx, "en", word); err != domain.ErrVocabNotFound || v != nil {
			t.Fatalf("%q must remain a real not-found result: v=%+v err=%v", word, v, err)
		}
	}
}

// QA N03: a lesson card saved with blank translations borrows the dictionary
// meaning, and blank values never erase an existing meaning.
func TestAddCardKeepsAndBorrowsMeaning(t *testing.T) {
	r := repo.NewMemorySRSRepo()
	svc := NewService(r, nil, func() time.Time { return schedClock })
	ctx := context.Background()
	_ = r.EnsureVocab(ctx, &domain.Vocab{Language: "en", Kind: "word", Lemma: "hi", Meaning: map[string]string{"fa": "سلام", "ckb": "سڵاو"}})
	if _, err := svc.AddCard(ctx, "u1", AddInput{Language: "en", Kind: "phrase", Lemma: "Hi!", Meaning: map[string]string{"fa": "", "ckb": ""}, Source: "lesson"}); err != nil {
		t.Fatal(err)
	}
	cards, err := svc.Due(ctx, "u1", "en", 10)
	if err != nil || len(cards) != 1 {
		t.Fatalf("due: %v %+v", err, cards)
	}
	if cards[0].Meaning["fa"] != "سلام" || cards[0].Meaning["ckb"] != "سڵاو" {
		t.Fatalf("review card has no native meaning: %+v", cards[0].Meaning)
	}
	// Re-saving the same card with blanks must not erase the meaning.
	_, _ = svc.AddCard(ctx, "u1", AddInput{Language: "en", Kind: "phrase", Lemma: "Hi!", Meaning: map[string]string{"fa": ""}, Source: "lesson"})
	cards, _ = svc.Due(ctx, "u1", "en", 10)
	if cards[0].Meaning["fa"] != "سلام" {
		t.Fatalf("blank re-save erased meaning: %+v", cards[0].Meaning)
	}
}

// A graduated card reviewed before it is due changes nothing and earns no XP;
// a due review earns XP once, and re-reviewing a state restored by Undo does
// not earn it again.
func TestEarlyReviewAndUndoDoNotFarmXP(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemorySRSRepo()
	rw := &xpCounter{}
	svc := NewService(r, rw, func() time.Time { return now })
	ctx := context.Background()
	c, err := svc.AddCard(ctx, "u1", AddInput{Language: "en", Lemma: "early", Meaning: map[string]string{"fa": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Review(ctx, "u1", c.ID, domain.GradeGood); err != nil || rw.xp != 2 {
		t.Fatalf("due review: xp=%d err=%v", rw.xp, err)
	}
	after, _ := r.Card(ctx, "u1", c.ID)
	for i := 0; i < 3; i++ {
		got, err := svc.Review(ctx, "u1", c.ID, domain.GradeEasy)
		if err != nil {
			t.Fatal(err)
		}
		if !got.DueAt.Equal(after.DueAt) || got.IntervalDays != after.IntervalDays {
			t.Fatal("an early review must not advance the schedule")
		}
	}
	if rw.xp != 2 {
		t.Fatalf("early reviews must not earn XP, xp=%d", rw.xp)
	}
	if _, err := svc.Undo(ctx, "u1", c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Review(ctx, "u1", c.ID, domain.GradeGood); err != nil {
		t.Fatal(err)
	}
	if rw.xp != 2 {
		t.Fatalf("review -> undo -> review must not earn XP twice, xp=%d", rw.xp)
	}
}

type xpCounter struct{ xp int }

func (x *xpCounter) Award(_ context.Context, _ string, xp int, _ string) error {
	x.xp += xp
	return nil
}
