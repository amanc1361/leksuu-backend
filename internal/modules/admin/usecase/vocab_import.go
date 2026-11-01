package usecase

import (
	"context"
	"strings"

	srsdomain "github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

const MaxVocabularyImportRows = 3000

type VocabularyImportRow struct {
	Language      string            `json:"language"`
	Kind          string            `json:"kind,omitempty"`
	Lemma         string            `json:"lemma"`
	Meaning       map[string]string `json:"meaning"`
	Example       string            `json:"example,omitempty"`
	Pronunciation string            `json:"pronunciation,omitempty"`
	Source        string            `json:"source"`
	Verified      bool              `json:"verified"`
}

type VocabularyImportResult struct {
	Index    int      `json:"index"`
	Lemma    string   `json:"lemma"`
	Status   string   `json:"status"`
	Missing  []string `json:"missing,omitempty"`
	Conflict []string `json:"conflict,omitempty"`
}

type VocabularyImportReport struct {
	DryRun      bool                     `json:"dry_run"`
	Reviewer    string                   `json:"reviewer"`
	Total       int                      `json:"total"`
	Created     int                      `json:"created"`
	Filled      int                      `json:"filled"`
	Unchanged   int                      `json:"unchanged"`
	NeedsReview int                      `json:"needs_review"`
	Conflicts   int                      `json:"conflicts"`
	Results     []VocabularyImportResult `json:"results"`
}

// ImportVocabulary is deliberately fill-only. A verified row may create a new
// entry or populate blank fields, but it can never replace a nonblank meaning.
// Dry-run and apply share this exact decision path so the preview is reliable.
func (s *Service) ImportVocabulary(ctx context.Context, reviewer string, dryRun bool, rows []VocabularyImportRow) (*VocabularyImportReport, error) {
	if s.vocab == nil || strings.TrimSpace(reviewer) == "" || len(rows) == 0 || len(rows) > MaxVocabularyImportRows {
		return nil, apperr.ErrBadRequest
	}
	report := &VocabularyImportReport{DryRun: dryRun, Reviewer: reviewer, Total: len(rows), Results: make([]VocabularyImportResult, 0, len(rows))}
	for index, row := range rows {
		result := s.importVocabularyRow(ctx, dryRun, index, row)
		report.Results = append(report.Results, result)
		switch result.Status {
		case "created":
			report.Created++
		case "filled":
			report.Filled++
		case "unchanged":
			report.Unchanged++
		case "conflict":
			report.Conflicts++
		default:
			report.NeedsReview++
		}
	}
	return report, nil
}

func (s *Service) importVocabularyRow(ctx context.Context, dryRun bool, index int, row VocabularyImportRow) VocabularyImportResult {
	language := strings.ToLower(strings.TrimSpace(row.Language))
	kind := strings.ToLower(strings.TrimSpace(row.Kind))
	if kind == "" {
		kind = "word"
	}
	lemma := srsdomain.NormalizeLemma(row.Lemma)
	result := VocabularyImportResult{Index: index, Lemma: lemma}
	meaning := cleanVocabularyMeanings(row.Meaning)
	if language == "" || lemma == "" || strings.TrimSpace(row.Source) == "" || !row.Verified {
		result.Status = "needs_review"
		return result
	}

	found, err := s.vocab.LookupVocab(ctx, language, lemma)
	if err != nil && err != srsdomain.ErrVocabNotFound {
		result.Status = "error"
		return result
	}
	var existing *srsdomain.Vocab
	if err == nil && found.Kind == kind && srsdomain.NormalizeLemma(found.Lemma) == lemma {
		existing = found
	}

	if existing == nil {
		result.Missing = missingLearnerMeanings(meaning)
		if len(result.Missing) > 0 {
			result.Status = "needs_review"
			return result
		}
		result.Status = "created"
		if dryRun {
			return result
		}
		v := &srsdomain.Vocab{Language: language, Kind: kind, Lemma: lemma, Meaning: meaning, Example: strings.TrimSpace(row.Example), Pronunciation: strings.TrimSpace(row.Pronunciation)}
		if err := s.vocab.EnsureVocab(ctx, v); err != nil {
			result.Status = "error"
		}
		return result
	}

	merged := make(map[string]string, len(existing.Meaning)+len(meaning))
	for locale, value := range existing.Meaning {
		merged[locale] = value
	}
	for locale, value := range meaning {
		if current := strings.TrimSpace(existing.Meaning[locale]); current != "" {
			if current != value {
				result.Conflict = append(result.Conflict, locale)
			}
			continue
		}
		merged[locale] = value
	}
	if len(result.Conflict) > 0 {
		result.Status = "conflict"
		return result
	}
	changed := false
	for locale, value := range merged {
		if strings.TrimSpace(existing.Meaning[locale]) == "" && strings.TrimSpace(value) != "" {
			changed = true
		}
	}
	if !changed {
		result.Status = "unchanged"
		return result
	}
	result.Status = "filled"
	if dryRun {
		return result
	}
	existing.Meaning = merged
	if existing.Example == "" {
		existing.Example = strings.TrimSpace(row.Example)
	}
	if existing.Pronunciation == "" {
		existing.Pronunciation = strings.TrimSpace(row.Pronunciation)
	}
	if err := s.vocab.UpdateVocab(ctx, existing); err != nil {
		result.Status = "error"
	}
	return result
}

func cleanVocabularyMeanings(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for locale, value := range in {
		if value = strings.TrimSpace(value); value != "" {
			out[strings.ToLower(strings.TrimSpace(locale))] = value
		}
	}
	return out
}

func missingLearnerMeanings(meaning map[string]string) []string {
	var missing []string
	for _, locale := range []string{"fa", "ckb"} {
		if strings.TrimSpace(meaning[locale]) == "" {
			missing = append(missing, locale)
		}
	}
	return missing
}
