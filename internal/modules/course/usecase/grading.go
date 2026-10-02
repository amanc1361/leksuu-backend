package usecase

import (
	"encoding/json"
	"strings"

	"github.com/leksuu/platform/internal/modules/course/domain"
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
)

// grade evaluates a learner's answer SERVER-SIDE against the exercise's answer
// key, returning whether it was correct and the correct solution (for feedback
// shown only after the attempt). The answer key never leaves the browse API.
func grade(ex *domain.Exercise, answer json.RawMessage) (bool, json.RawMessage) {
	switch ex.Type {
	case TypeMCQ, TypeListenChoose:
		var key struct {
			Correct int `json:"correct"`
		}
		var ans struct {
			Choice int `json:"choice"`
		}
		_ = json.Unmarshal(ex.AnswerKey, &key)
		_ = json.Unmarshal(answer, &ans)
		sol, _ := json.Marshal(map[string]int{"choice": key.Correct})
		return ans.Choice == key.Correct, sol

	case TypeFillBlank, TypeDictation:
		var key struct {
			Answers []string `json:"answers"`
		}
		var ans struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(ex.AnswerKey, &key)
		_ = json.Unmarshal(answer, &ans)
		correct := false
		for _, a := range key.Answers {
			if normalize(a) == normalize(ans.Text) {
				correct = true
				break
			}
		}
		var first string
		if len(key.Answers) > 0 {
			first = key.Answers[0]
		}
		sol, _ := json.Marshal(map[string]string{"text": first})
		return correct, sol

	case TypeReorder, TypeSentenceBuild:
		var key struct {
			Order []string `json:"order"`
		}
		var ans struct {
			Order []string `json:"order"`
		}
		_ = json.Unmarshal(ex.AnswerKey, &key)
		_ = json.Unmarshal(answer, &ans)
		sol, _ := json.Marshal(map[string][]string{"order": key.Order})
		return equalSeq(key.Order, ans.Order), sol

	case TypeMatch, TypeMatchImage:
		var key struct {
			Pairs [][]string `json:"pairs"`
		}
		var ans struct {
			Pairs [][]string `json:"pairs"`
		}
		_ = json.Unmarshal(ex.AnswerKey, &key)
		_ = json.Unmarshal(answer, &ans)
		sol, _ := json.Marshal(map[string][][]string{"pairs": key.Pairs})
		return samePairs(key.Pairs, ans.Pairs), sol

	case TypeTrueFalse:
		var key struct {
			Answer bool `json:"answer"`
		}
		var ans struct {
			Value bool `json:"value"`
		}
		_ = json.Unmarshal(ex.AnswerKey, &key)
		_ = json.Unmarshal(answer, &ans)
		sol, _ := json.Marshal(map[string]bool{"answer": key.Answer})
		return ans.Value == key.Answer, sol

	case TypeSelectWords:
		// Order-free: the learner must pick exactly the keyed tiles, no more.
		var key struct {
			Selected []int `json:"selected"`
		}
		var ans struct {
			Selected []int `json:"selected"`
		}
		_ = json.Unmarshal(ex.AnswerKey, &key)
		_ = json.Unmarshal(answer, &ans)
		sol, _ := json.Marshal(map[string][]int{"selected": key.Selected})
		return sameIndexSet(key.Selected, ans.Selected), sol

	case TypeSpeak:
		// The client records audio, scores it via the pronunciation service, and
		// submits the achieved overall score. Pass if it meets the minimum.
		var key struct {
			MinScore int `json:"min_score"`
		}
		var ans struct {
			Score int `json:"score"`
		}
		_ = json.Unmarshal(ex.AnswerKey, &key)
		_ = json.Unmarshal(answer, &ans)
		min := key.MinScore
		if min <= 0 {
			min = 60
		}
		sol, _ := json.Marshal(map[string]int{"min_score": min})
		return ans.Score >= min, sol
	}
	return false, nil
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
