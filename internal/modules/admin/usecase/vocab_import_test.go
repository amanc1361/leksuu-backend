package usecase

import (
	"context"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
	"github.com/leksuu/platform/internal/modules/srs/domain"
	srsrepo "github.com/leksuu/platform/internal/modules/srs/repo"
)

func TestVocabularyImportDryRunAndFillOnlyApply(t *testing.T) {
	ctx := context.Background()
	vocab := srsrepo.NewMemorySRSRepo()
	if err := vocab.EnsureVocab(ctx, &domain.Vocab{
		Language: "en", Kind: "word", Lemma: "and",
		Meaning: map[string]string{"fa": "و"},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(courserepo.NewMemoryCourseRepo(), vocab)
	rows := []VocabularyImportRow{
		{Language: "en", Lemma: "And,", Meaning: map[string]string{"ckb": "و"}, Source: "teen-adult-review", Verified: true},
		{Language: "en", Lemma: "expression", Meaning: map[string]string{"fa": "عبارت", "ckb": "دەربڕین"}, Source: "teen-adult-review", Verified: true},
		{Language: "en", Lemma: "informal", Meaning: map[string]string{"fa": "غیررسمی", "ckb": "نافەرمی"}, Source: "teen-adult-draft", Verified: false},
	}

	preview, err := svc.ImportVocabulary(ctx, "reviewer-1", true, rows)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Filled != 1 || preview.Created != 1 || preview.NeedsReview != 1 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	andBefore, _ := vocab.LookupVocab(ctx, "en", "and")
	if andBefore.Meaning["ckb"] != "" {
		t.Fatal("dry run changed the existing entry")
	}
	if _, err := vocab.LookupVocab(ctx, "en", "expression"); err != domain.ErrVocabNotFound {
		t.Fatal("dry run created a vocabulary row")
	}

	applied, err := svc.ImportVocabulary(ctx, "reviewer-1", false, rows)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Filled != 1 || applied.Created != 1 || applied.NeedsReview != 1 {
		t.Fatalf("unexpected apply report: %+v", applied)
	}
	andAfter, _ := vocab.LookupVocab(ctx, "en", "and")
	if andAfter.Meaning["fa"] != "و" || andAfter.Meaning["ckb"] != "و" {
		t.Fatalf("fill-only merge failed: %+v", andAfter.Meaning)
	}
	created, err := vocab.LookupVocab(ctx, "en", "expression")
	if err != nil || created.Meaning["fa"] == "" || created.Meaning["ckb"] == "" {
		t.Fatalf("verified complete row was not created: v=%+v err=%v", created, err)
	}
	if _, err := vocab.LookupVocab(ctx, "en", "informal"); err != domain.ErrVocabNotFound {
		t.Fatal("unverified row was imported")
	}
}

func TestVocabularyImportReportsConflictWithoutOverwrite(t *testing.T) {
	ctx := context.Background()
	vocab := srsrepo.NewMemorySRSRepo()
	if err := vocab.EnsureVocab(ctx, &domain.Vocab{
		Language: "en", Kind: "word", Lemma: "the",
		Meaning: map[string]string{"fa": "حرف تعریف معین", "ckb": "ئامرازی ناسراو"},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(courserepo.NewMemoryCourseRepo(), vocab)
	report, err := svc.ImportVocabulary(ctx, "reviewer-2", false, []VocabularyImportRow{{
		Language: "en", Lemma: "the", Source: "teen-adult-review", Verified: true,
		Meaning: map[string]string{"fa": "یک معنی متفاوت", "ckb": "ئامرازی ناسراو"},
	}})
	if err != nil || report.Conflicts != 1 {
		t.Fatalf("conflict was not reported: report=%+v err=%v", report, err)
	}
	entry, _ := vocab.LookupVocab(ctx, "en", "the")
	if entry.Meaning["fa"] != "حرف تعریف معین" {
		t.Fatalf("existing meaning was overwritten: %+v", entry.Meaning)
	}
}
