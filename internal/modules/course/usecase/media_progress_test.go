package usecase

import "testing"

func TestMediaAnswerNormalizationMatchesApprovedTypingRules(t *testing.T) {
	if normalizeMediaAnswer("Café, HELLO!", "en") != normalizeMediaAnswer("cafe hello", "en") {
		t.Fatal("English accent, case and punctuation should not change grading")
	}
	if normalizeMediaAnswer("كيان؟", "fa") != normalizeMediaAnswer("کیان", "fa") {
		t.Fatal("Arabic/Persian keyboard variants should not change grading")
	}
}
