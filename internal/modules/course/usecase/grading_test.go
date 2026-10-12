package usecase

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/leksuu/platform/internal/modules/course/domain"
)

func ex(t, key string) *domain.Exercise {
	return &domain.Exercise{Type: t, AnswerKey: json.RawMessage(key)}
}

func exWithPrompt(t, prompt, key string) *domain.Exercise {
	return &domain.Exercise{Type: t, Prompt: json.RawMessage(prompt), AnswerKey: json.RawMessage(key)}
}

func TestGradeAllTypes(t *testing.T) {
	cases := []struct {
		name   string
		ex     *domain.Exercise
		answer string
		want   bool
	}{
		{"mcq right", ex("mcq", `{"correct":1}`), `{"choice":1}`, true},
		{"mcq wrong", ex("mcq", `{"correct":1}`), `{"choice":0}`, false},
		{"mcq choice 0 is a real answer", ex("mcq", `{"correct":0}`), `{"choice":0}`, true},
		{"fill right (case/space/punct insensitive)", ex("fill_blank", `{"answers":["are"]}`), `{"text":"  Are. "}`, true},
		{"fill wrong", ex("fill_blank", `{"answers":["are"]}`), `{"text":"is"}`, false},
		{"reorder right", ex("reorder", `{"order":["I","am","Sara"]}`), `{"order":["I","am","Sara"]}`, true},
		{"reorder wrong", ex("reorder", `{"order":["I","am","Sara"]}`), `{"order":["am","I","Sara"]}`, false},
		{"match right (any order)", ex("match", `{"pairs":[["Hello","سلام"],["Bye","خداحافظ"]]}`), `{"pairs":[["Bye","خداحافظ"],["Hello","سلام"]]}`, true},
		{"match wrong", ex("match", `{"pairs":[["Hello","سلام"],["Bye","خداحافظ"]]}`), `{"pairs":[["Hello","خداحافظ"],["Bye","سلام"]]}`, false},
		// "sie" (she / they) vs "Sie" (formal you): case is the lesson.
		{"match sie/Sie right", ex("match", `{"pairs":[["sie","she"],["sie","they"],["Sie","you (formal)"]]}`), `{"pairs":[["Sie","you (formal)"],["sie","they"],["sie","she"]]}`, true},
		{"match sie/Sie case swapped", ex("match", `{"pairs":[["sie","she"],["sie","they"],["Sie","you (formal)"]]}`), `{"pairs":[["Sie","she"],["sie","they"],["sie","you (formal)"]]}`, false},
		{"match tolerates extra spaces", ex("match", `{"pairs":[["Guten Tag","Hello"]]}`), `{"pairs":[[" Guten  Tag ","Hello"]]}`, true},
		{"true/false right (false)", ex("true_false", `{"answer":false}`), `{"value":false}`, true},
		{"true/false wrong", ex("true_false", `{"answer":false}`), `{"value":true}`, false},
		{"select right (any order)", ex("select_words", `{"selected":[0,4]}`), `{"selected":[4,0]}`, true},
		{"select missing one", ex("select_words", `{"selected":[0,4]}`), `{"selected":[0]}`, false},
		{"select extra tile", ex("select_words", `{"selected":[0,4]}`), `{"selected":[0,4,2]}`, false},
		{"select wrong tile", ex("select_words", `{"selected":[0,4]}`), `{"selected":[0,3]}`, false},
		{"fill accepts an alternative", ex("fill_blank", `{"answers":["aren't","are not"]}`), `{"text":"are not"}`, true},
		// fill_gaps — ordered multi-blank, every gap must match (N01)
		{"gaps all correct", ex("fill_gaps", `{"gaps":[["'m","am"],["are"]]}`), `{"gaps":["am","Are."]}`, true},
		{"gaps second wrong", ex("fill_gaps", `{"gaps":[["'m","am"],["are"]]}`), `{"gaps":["'m","is"]}`, false},
		{"gaps swapped", ex("fill_gaps", `{"gaps":[["'m","am"],["are"]]}`), `{"gaps":["are","'m"]}`, false},

		{"gaps all right", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"gaps":["'m","are"]}`, true},
		{"gaps second wrong", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"gaps":["'m","be"]}`, false},
		{"gaps first wrong", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"gaps":["'s","are"]}`, false},
		{"gaps swapped", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"gaps":["are","'m"]}`, false},
		{"gaps accept a variant per gap", ex("fill_gaps", `{"gaps":[["aren't","are not"],["here"]]}`), `{"gaps":["are not","Here"]}`, true},
		{"gaps repeated word", ex("fill_gaps", `{"gaps":[["the"],["the"]]}`), `{"gaps":["the","the"]}`, true},
		{"tile gaps are case-exact", ex("fill_gaps", `{"gaps":[["him"],["He"]],"exact":true}`), `{"gaps":["him","he"]}`, false},
		{"tile gaps right", ex("fill_gaps", `{"gaps":[["him"],["He"]],"exact":true}`), `{"gaps":["him"," He "]}`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, sol, err := grade(c.ex, json.RawMessage(c.answer))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("grade=%v want=%v", got, c.want)
			}
			if len(sol) == 0 {
				t.Fatal("expected a solution for feedback")
			}
		})
	}
}

// A missing, malformed or wrong-shaped answer is rejected before grading — it
// can never count as correct through a zero value ({} == choice 0 / false).
func TestGradeRejectsInvalidAnswers(t *testing.T) {
	cases := []struct {
		name   string
		ex     *domain.Exercise
		answer string
	}{
		{"mcq empty object", ex("mcq", `{"correct":0}`), `{}`},
		{"mcq null", ex("mcq", `{"correct":0}`), `null`},
		{"mcq no body", ex("mcq", `{"correct":0}`), ``},
		{"mcq malformed", ex("mcq", `{"correct":0}`), `{"choice":`},
		{"mcq wrong type", ex("mcq", `{"correct":0}`), `{"choice":"0"}`},
		{"mcq negative", ex("mcq", `{"correct":0}`), `{"choice":-1}`},
		{"mcq out of range", exWithPrompt("mcq", `{"options":["a","b"]}`, `{"correct":0}`), `{"choice":2}`},
		{"mcq unknown field", ex("mcq", `{"correct":0}`), `{"choice":0,"cheat":true}`},
		{"true/false empty", ex("true_false", `{"answer":false}`), `{}`},
		{"fill empty text", ex("fill_blank", `{"answers":["are"]}`), `{"text":"   "}`},
		{"fill missing text", ex("fill_blank", `{"answers":["are"]}`), `{}`},
		{"reorder empty", ex("reorder", `{"order":["a","b"]}`), `{"order":[]}`},
		{"match bad pair", ex("match", `{"pairs":[["a","b"]]}`), `{"pairs":[["a"]]}`},
		{"select duplicate tile", ex("select_words", `{"selected":[0,1]}`), `{"selected":[0,0]}`},
		{"select out of range", exWithPrompt("select_words", `{"tokens":["a","b"]}`, `{"selected":[0]}`), `{"selected":[5]}`},
		{"gaps one answer for two gaps", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"gaps":["'m"]}`},
		{"gaps second left blank", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"gaps":["'m",""]}`},
		{"gaps missing", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{}`},
		{"gaps too many", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"gaps":["'m","are","x"]}`},
		{"mcq-shaped answer to gaps", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"choice":0}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, _, err := grade(c.ex, json.RawMessage(c.answer))
			if !errors.Is(err, ErrInvalidAnswer) {
				t.Fatalf("want ErrInvalidAnswer, got ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestGradeMisconfiguredKey(t *testing.T) {
	for _, e := range []*domain.Exercise{
		ex("mcq", `{}`), ex("fill_blank", `{"answers":[]}`), ex("fill_gaps", `{"gaps":[[]]}`), ex("nope", `{}`),
	} {
		if _, _, err := grade(e, json.RawMessage(`{"choice":0}`)); !errors.Is(err, ErrExerciseMisconfigured) {
			t.Fatalf("%s %s: want ErrExerciseMisconfigured, got %v", e.Type, e.AnswerKey, err)
		}
	}
}

func TestGradeSpeak(t *testing.T) {
	e := ex(TypeSpeak, `{"min_score":70}`)
	if ok, _, _ := gradeSpeak(e, 85); !ok {
		t.Fatal("score 85 >= 70 should pass")
	}
	if ok, _, _ := gradeSpeak(e, 60); ok {
		t.Fatal("score 60 < 70 should fail")
	}
	if ok, _, _ := gradeSpeak(ex(TypeSpeak, `{}`), 60); !ok {
		t.Fatal("score 60 >= default 60 should pass")
	}
	if _, _, err := gradeSpeak(e, 999999); !errors.Is(err, ErrInvalidAnswer) {
		t.Fatalf("a score above 100 must be rejected, got %v", err)
	}
}
