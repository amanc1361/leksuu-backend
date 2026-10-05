package usecase

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Supported interactive exercise types (mirror the DB CHECK + the frontend renderers).
const (
	TypeMCQ           = "mcq"
	TypeFillBlank     = "fill_blank"
	TypeReorder       = "reorder"
	TypeMatch         = "match"
	TypeListenChoose  = "listen_choose"    // audio prompt + choose (listening) — graded like MCQ
	TypeTrueFalse     = "true_false"       // statement → true/false (listening/reading)
	TypeDictation     = "dictation"        // type what you hear (listening/writing) — graded like fill_blank
	TypeSentenceBuild = "sentence_builder" // arrange word tiles (writing) — graded like reorder
	TypeMatchImage    = "match_image"      // match word ↔ image (vocab) — graded like match
	TypeSpeak         = "speak"            // say a phrase aloud → pronunciation score (Phase 4)
	TypeSelectWords   = "select_words"     // pick every correct tile from a set (grammar spotting)
	TypeFillGaps      = "fill_gaps"        // a sentence with several gaps; every gap must be right
)

var (
	// ErrInvalidAnswer: the submitted answer is missing, malformed, the wrong
	// shape for the exercise, or out of range. It is rejected before grading,
	// so it is never recorded as an attempt and can never count as correct.
	ErrInvalidAnswer = apperr.New("exercise.invalid_answer", apperr.KindValidation)
	// ErrExerciseMisconfigured: the stored answer key cannot be graded (an
	// unknown type or a malformed key). A content problem, never the learner's.
	ErrExerciseMisconfigured = apperr.New("exercise.misconfigured", apperr.KindInternal)
)

// decodeAnswer strictly decodes a learner answer: an absent, null or malformed
// body, or one with unknown fields, is invalid.
func decodeAnswer(raw json.RawMessage, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ErrInvalidAnswer
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ErrInvalidAnswer
	}
	return nil
}

// decodeKey decodes a stored answer key; a malformed key is a content error.
func decodeKey(raw json.RawMessage, v any) error {
	if len(raw) == 0 || json.Unmarshal(raw, v) != nil {
		return ErrExerciseMisconfigured
	}
	return nil
}

// promptLen returns the length of a list in the prompt (e.g. "options",
// "tokens"), or -1 when the prompt does not carry it.
func promptLen(prompt json.RawMessage, field string) int {
	var p map[string]json.RawMessage
	if json.Unmarshal(prompt, &p) != nil {
		return -1
	}
	var list []json.RawMessage
	if raw, ok := p[field]; !ok || json.Unmarshal(raw, &list) != nil {
		return -1
	}
	return len(list)
}

// grade evaluates a learner's answer SERVER-SIDE against the exercise's answer
// key, returning whether it was correct and the correct solution (for feedback
// shown only after the attempt). The answer key never leaves the browse API.
// Speak exercises are not graded here: their score comes from the server's
// pronunciation assessment (see gradeSpeak).
func grade(ex *domain.Exercise, answer json.RawMessage) (bool, json.RawMessage, error) {
	switch ex.Type {
	case TypeMCQ, TypeListenChoose:
		var key struct {
			Correct *int `json:"correct"`
		}
		var ans struct {
			Choice *int `json:"choice"`
		}
		if err := decodeKey(ex.AnswerKey, &key); err != nil || key.Correct == nil {
			return false, nil, ErrExerciseMisconfigured
		}
		if err := decodeAnswer(answer, &ans); err != nil || ans.Choice == nil || *ans.Choice < 0 {
			return false, nil, ErrInvalidAnswer
		}
		if n := promptLen(ex.Prompt, "options"); n >= 0 && *ans.Choice >= n {
			return false, nil, ErrInvalidAnswer
		}
		sol, _ := json.Marshal(map[string]int{"choice": *key.Correct})
		return *ans.Choice == *key.Correct, sol, nil

	case TypeFillBlank, TypeDictation:
		var key struct {
			Answers []string `json:"answers"`
		}
		var ans struct {
			Text *string `json:"text"`
		}
		if err := decodeKey(ex.AnswerKey, &key); err != nil || len(key.Answers) == 0 {
			return false, nil, ErrExerciseMisconfigured
		}
		if err := decodeAnswer(answer, &ans); err != nil || ans.Text == nil || strings.TrimSpace(*ans.Text) == "" {
			return false, nil, ErrInvalidAnswer
		}
		sol, _ := json.Marshal(map[string]string{"text": key.Answers[0]})
		return acceptable(key.Answers, *ans.Text), sol, nil

	case TypeReorder, TypeSentenceBuild:
		var key struct {
			Order []string `json:"order"`
		}
		var ans struct {
			Order []string `json:"order"`
		}
		if err := decodeKey(ex.AnswerKey, &key); err != nil || len(key.Order) == 0 {
			return false, nil, ErrExerciseMisconfigured
		}
		if err := decodeAnswer(answer, &ans); err != nil || len(ans.Order) == 0 {
			return false, nil, ErrInvalidAnswer
		}
		sol, _ := json.Marshal(map[string][]string{"order": key.Order})
		return equalSeq(key.Order, ans.Order), sol, nil

	case TypeMatch, TypeMatchImage:
		var key struct {
			Pairs [][]string `json:"pairs"`
		}
		var ans struct {
			Pairs [][]string `json:"pairs"`
		}
		if err := decodeKey(ex.AnswerKey, &key); err != nil || len(key.Pairs) == 0 {
			return false, nil, ErrExerciseMisconfigured
		}
		if err := decodeAnswer(answer, &ans); err != nil || len(ans.Pairs) == 0 {
			return false, nil, ErrInvalidAnswer
		}
		for _, p := range ans.Pairs {
			if len(p) != 2 {
				return false, nil, ErrInvalidAnswer
			}
		}
		sol, _ := json.Marshal(map[string][][]string{"pairs": key.Pairs})
		return samePairs(key.Pairs, ans.Pairs), sol, nil

	case TypeTrueFalse:
		var key struct {
			Answer *bool `json:"answer"`
		}
		var ans struct {
			Value *bool `json:"value"`
		}
		if err := decodeKey(ex.AnswerKey, &key); err != nil || key.Answer == nil {
			return false, nil, ErrExerciseMisconfigured
		}
		if err := decodeAnswer(answer, &ans); err != nil || ans.Value == nil {
			return false, nil, ErrInvalidAnswer
		}
		sol, _ := json.Marshal(map[string]bool{"answer": *key.Answer})
		return *ans.Value == *key.Answer, sol, nil

	case TypeSelectWords:
		// Order-free: the learner must pick exactly the keyed tiles, no more.
		var key struct {
			Selected []int `json:"selected"`
		}
		var ans struct {
			Selected []int `json:"selected"`
		}
		if err := decodeKey(ex.AnswerKey, &key); err != nil || len(key.Selected) == 0 {
			return false, nil, ErrExerciseMisconfigured
		}
		if err := decodeAnswer(answer, &ans); err != nil || len(ans.Selected) == 0 {
			return false, nil, ErrInvalidAnswer
		}
		n := promptLen(ex.Prompt, "tokens")
		seen := map[int]bool{}
		for _, i := range ans.Selected {
			if i < 0 || (n >= 0 && i >= n) || seen[i] {
				return false, nil, ErrInvalidAnswer
			}
			seen[i] = true
		}
		sol, _ := json.Marshal(map[string][]int{"selected": key.Selected})
		return sameIndexSet(key.Selected, ans.Selected), sol, nil

	case TypeFillGaps:
		// Every gap is answered independently and every gap must be right: one
		// correct gap can never satisfy the whole sentence. key.gaps[i] lists
		// the accepted answers of gap i ("aren't" | "are not"). With exact set
		// (answers picked from tiles) the tile text must match as written:
		// tiles may differ only in capitals ("His" vs "his") on purpose.
		var key struct {
			Gaps  [][]string `json:"gaps"`
			Exact bool       `json:"exact"`
		}
		var ans struct {
			Gaps []string `json:"gaps"`
		}
		if err := decodeKey(ex.AnswerKey, &key); err != nil || len(key.Gaps) == 0 {
			return false, nil, ErrExerciseMisconfigured
		}
		for _, g := range key.Gaps {
			if len(g) == 0 {
				return false, nil, ErrExerciseMisconfigured
			}
		}
		if err := decodeAnswer(answer, &ans); err != nil || len(ans.Gaps) != len(key.Gaps) {
			return false, nil, ErrInvalidAnswer
		}
		for _, a := range ans.Gaps {
			if strings.TrimSpace(a) == "" {
				return false, nil, ErrInvalidAnswer
			}
		}
		correct := true
		first := make([]string, len(key.Gaps))
		for i, accepted := range key.Gaps {
			first[i] = accepted[0]
			ok := acceptable(accepted, ans.Gaps[i])
			if key.Exact {
				ok = acceptableExact(accepted, ans.Gaps[i])
			}
			if !ok {
				correct = false
			}
		}
		sol, _ := json.Marshal(map[string][]string{"gaps": first})
		return correct, sol, nil
	}
	return false, nil, ErrExerciseMisconfigured
}

// speakMinScore is the pass mark of a speak exercise (default 60 of 100).
func speakMinScore(ex *domain.Exercise) int {
	var key struct {
		MinScore int `json:"min_score"`
	}
	_ = json.Unmarshal(ex.AnswerKey, &key)
	if key.MinScore <= 0 || key.MinScore > 100 {
		return 60
	}
	return key.MinScore
}

// gradeSpeak grades a speak exercise from a score the server itself produced
// (the learner's latest pronunciation assessment of this exercise). Scores
// outside 0..100 are rejected.
func gradeSpeak(ex *domain.Exercise, score int) (bool, json.RawMessage, error) {
	if score < 0 || score > 100 {
		return false, nil, ErrInvalidAnswer
	}
	min := speakMinScore(ex)
	sol, _ := json.Marshal(map[string]int{"min_score": min})
	return score >= min, sol, nil
}

func acceptable(accepted []string, answer string) bool {
	for _, a := range accepted {
		if normalize(a) == normalize(answer) {
			return true
		}
	}
	return false
}

// acceptableExact compares as written, ignoring only surrounding and repeated
// whitespace.
func acceptableExact(accepted []string, answer string) bool {
	a := strings.Join(strings.Fields(answer), " ")
	for _, want := range accepted {
		if strings.Join(strings.Fields(want), " ") == a {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimRight(s, ".!?؟،,")
	return strings.Join(strings.Fields(s), " ")
}

func equalSeq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if normalize(a[i]) != normalize(b[i]) {
			return false
		}
	}
	return true
}

func sameIndexSet(a, b []int) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	set := map[int]int{}
	for _, i := range a {
		set[i]++
	}
	for _, i := range b {
		set[i]--
	}
	for _, v := range set {
		if v != 0 {
			return false
		}
	}
	return true
}

func samePairs(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]int{}
	keyOf := func(p []string) string {
		if len(p) < 2 {
			return ""
		}
		return normalize(p[0]) + "\x00" + normalize(p[1])
	}
	for _, p := range a {
		set[keyOf(p)]++
	}
	for _, p := range b {
		set[keyOf(p)]--
	}
	for _, v := range set {
		if v != 0 {
			return false
		}
	}
	return true
}
