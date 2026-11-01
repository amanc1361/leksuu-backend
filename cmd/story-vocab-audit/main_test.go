package main

import "testing"

func TestCanonicalLemmaKeepsGrammarAndNormalizesCommonInflections(t *testing.T) {
	cases := map[string]string{
		"the": "the", "i": "i", "and": "and",
		"expressions": "expression", "celebrated": "celebrate",
		"noted": "note", "making": "make", "stopped": "stop",
		"were": "be", "children": "child", "he's": "he's",
	}
	for input, want := range cases {
		if got := canonicalLemma(input); got != want {
			t.Errorf("canonicalLemma(%q)=%q want %q", input, got, want)
		}
	}
}

func TestImportTemplateNeverClaimsReview(t *testing.T) {
	rows := []lemmaReport{
		{Lemma: "and", Status: "missing_entry", Contexts: []string{"A and B."}},
		{Lemma: "informal", Status: "missing_ckb", Meaning: map[string]string{"fa": "غیررسمی"}},
		{Lemma: "Simon", Status: "proper_noun", ProperNoun: true},
		{Lemma: "hello", Status: "complete"},
	}
	template := buildTemplate("2026-10-28T00:00:00Z", rows)
	if len(template.Items) != 2 {
		t.Fatalf("template rows=%d want 2", len(template.Items))
	}
	for _, row := range template.Items {
		if row.Verified {
			t.Fatalf("generated row %q was incorrectly marked verified", row.Lemma)
		}
	}
	if len(template.Items[0].Needs) != 2 || len(template.Items[1].Needs) != 1 || template.Items[1].Needs[0] != "ckb" {
		t.Fatalf("wrong missing-locale instructions: %+v", template.Items)
	}
}
