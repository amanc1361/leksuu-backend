package domain

import (
	"encoding/json"
	"strings"
)

// Localizable string kinds of a grammar item.
const (
	L10nRule  = "rule"  // the explanation (native-language content)
	L10nTitle = "title" // the tip's own heading
	L10nTable = "table" // a table cell; only labels ("more formal") should get a translation
)

// LocalizableString is one source string of an item that may be shown in the
// learner's language.
type LocalizableString struct {
	Kind string
	Text string
	Lang string // source language
}

type grammarForm struct {
	Subject      string `json:"subject"`
	SubjectAudio string `json:"subject_audio"` // set when the subject is spoken target-language text
	Form         string `json:"form"`
	Audio        string `json:"audio"`
}

type grammarPayload struct {
	Title      string            `json:"title"`
	SourceLang string            `json:"source_lang"`
	Rule       map[string]string `json:"rule"`
	Forms      []grammarForm     `json:"forms"`
}

func (p grammarPayload) lang() string {
	if p.SourceLang != "" {
		return p.SourceLang
	}
	return "en"
}

// LocalizableStrings lists the source strings of an item that a translation
// can be attached to. Only grammar items carry explanatory text today; target
// language examples (forms with audio, examples) are never listed, so they
// always stay in the language being taught.
func LocalizableStrings(kind string, payload json.RawMessage) []LocalizableString {
	if kind != KindGrammar || len(payload) == 0 {
		return nil
	}
	var p grammarPayload
	if json.Unmarshal(payload, &p) != nil {
		return nil
	}
	lang := p.lang()
	var out []LocalizableString
	add := func(k, text string) {
		if t := strings.TrimSpace(text); t != "" {
			out = append(out, LocalizableString{Kind: k, Text: t, Lang: lang})
		}
	}
	add(L10nRule, p.Rule[lang])
	add(L10nTitle, p.Title)
	for _, f := range p.Forms {
		if f.SubjectAudio == "" {
			add(L10nTable, f.Subject)
		}
		if f.Audio == "" {
			add(L10nTable, f.Form)
		}
	}
	return out
}

// LocalizePayload adds stored translations to a grammar payload without
// touching its source text: rule[locale] is filled only where the payload has
// no text of its own for that locale, and title/table translations go into
// title_tr / subject_tr / form_tr. tr maps source hash -> locale -> text.
func LocalizePayload(kind string, payload json.RawMessage, tr map[string]map[string]string) json.RawMessage {
	if kind != KindGrammar || len(payload) == 0 || len(tr) == 0 {
		return payload
	}
	var src grammarPayload
	var doc map[string]any
	if json.Unmarshal(payload, &src) != nil || json.Unmarshal(payload, &doc) != nil {
		return payload
	}
	lang := src.lang()
	changed := false
	lookup := func(text string) map[string]string {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return tr[SourceHash(text)]
	}
	if rule, ok := doc["rule"].(map[string]any); ok {
		for loc, text := range lookup(src.Rule[lang]) {
			if _, own := rule[loc]; !own && loc != lang {
				rule[loc] = text
				changed = true
			}
		}
	}
	if found := lookup(src.Title); len(found) > 0 {
		doc["title_tr"] = found
		changed = true
	}
	if forms, ok := doc["forms"].([]any); ok {
		for i, raw := range forms {
			f, ok := raw.(map[string]any)
			if !ok || i >= len(src.Forms) {
				continue
			}
			if found := lookup(src.Forms[i].Subject); len(found) > 0 && src.Forms[i].SubjectAudio == "" {
				f["subject_tr"] = found
				changed = true
			}
			if src.Forms[i].Audio == "" {
				if found := lookup(src.Forms[i].Form); len(found) > 0 {
					f["form_tr"] = found
					changed = true
				}
			}
		}
	}
	if !changed {
		return payload
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return payload
	}
	return out
}
