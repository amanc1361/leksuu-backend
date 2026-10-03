package usecase

import (
	"context"
	"strings"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// TranslatableString is one source string of a course that can be shown in a
// learner locale, with its current translation (if any).
type TranslatableString struct {
	Hash        string `json:"hash"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	SourceLang  string `json:"source_lang"`
	Uses        int    `json:"uses"`
	Translation string `json:"translation"`
	Origin      string `json:"origin,omitempty"`
}

// translationLocales are the learner locales content can be translated into.
var translationLocales = map[string]bool{"fa": true, "ckb": true, "en": true}

// CourseTranslations lists the distinct localizable strings of a course (in
// course order) with their translation into locale.
func (s *Service) CourseTranslations(ctx context.Context, courseID, locale string) ([]TranslatableString, error) {
	if !translationLocales[locale] {
		return nil, apperr.ErrBadRequest
	}
	if _, err := s.repo.Course(ctx, courseID); err != nil {
		return nil, err
	}
	lessons, err := s.repo.Lessons(ctx, courseID)
	if err != nil {
		return nil, err
	}
	var out []TranslatableString
	index := map[string]int{}
	for _, l := range lessons {
		items, err := s.repo.Items(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			for _, ls := range coursedomain.LocalizableStrings(it.Kind, it.Payload) {
				h := coursedomain.SourceHash(ls.Text)
				if i, ok := index[h]; ok {
					out[i].Uses++
					continue
				}
				index[h] = len(out)
				out = append(out, TranslatableString{Hash: h, Kind: ls.Kind, Source: ls.Text, SourceLang: ls.Lang, Uses: 1})
			}
		}
	}
	hashes := make([]string, 0, len(out))
	for _, t := range out {
		hashes = append(hashes, t.Hash)
	}
	stored, err := s.repo.ContentTranslations(ctx, hashes)
	if err != nil {
		return nil, err
	}
	for _, t := range stored {
		if i, ok := index[t.SourceHash]; ok && t.Locale == locale {
			out[i].Translation, out[i].Origin = t.Text, t.Origin
		}
	}
	return out, nil
}

// TranslationInput is one translation an admin saves. An empty Text removes
// the translation (the learner then sees the source text again).
type TranslationInput struct {
	Source     string `json:"source"`
	SourceLang string `json:"source_lang"`
	Text       string `json:"text"`
	Origin     string `json:"origin"`
}

// maxTranslationsPerSave bounds one save request.
const maxTranslationsPerSave = 1000

// SaveTranslations upserts (or, for empty text, deletes) translations of
// source strings into locale. The source text itself is never modified.
func (s *Service) SaveTranslations(ctx context.Context, locale string, in []TranslationInput) (saved, deleted int, err error) {
	if !translationLocales[locale] || len(in) == 0 || len(in) > maxTranslationsPerSave {
		return 0, 0, apperr.ErrBadRequest
	}
	var upserts []coursedomain.ContentTranslation
	for _, t := range in {
		source := strings.TrimSpace(t.Source)
		if source == "" {
			return 0, 0, apperr.ErrBadRequest
		}
		hash := coursedomain.SourceHash(source)
		text := strings.TrimSpace(t.Text)
		if text == "" {
			if err := s.repo.DeleteContentTranslation(ctx, hash, locale); err != nil {
				return 0, 0, err
			}
			deleted++
			continue
		}
		origin := t.Origin
		switch origin {
		case coursedomain.OriginHuman, coursedomain.OriginMachine, coursedomain.OriginSource:
		case "":
			origin = coursedomain.OriginHuman
		default:
			return 0, 0, apperr.ErrBadRequest
		}
		lang := strings.TrimSpace(t.SourceLang)
		if lang == "" {
			lang = "en"
		}
		upserts = append(upserts, coursedomain.ContentTranslation{SourceHash: hash, Locale: locale, SourceLang: lang, SourceText: source, Text: text, Origin: origin})
	}
	if len(upserts) > 0 {
		if err := s.repo.UpsertContentTranslations(ctx, upserts); err != nil {
			return 0, 0, err
		}
	}
	return len(upserts), deleted, nil
}
