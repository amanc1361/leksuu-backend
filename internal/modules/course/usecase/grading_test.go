package usecase

import (
	"encoding/json"
	"testing"

	"github.com/leksuu/platform/internal/modules/course/domain"
)

func ex(t, key string) *domain.Exercise {
	return &domain.Exercise{Type: t, AnswerKey: json.RawMessage(key)}
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
		{"fill right (case/space/punct insensitive)", ex("fill_blank", `{"answers":["are"]}`), `{"text":"  Are. "}`, true},
		{"fill wrong", ex("fill_blank", `{"answers":["are"]}`), `{"text":"is"}`, false},
		{"reorder right", ex("reorder", `{"order":["I","am","Sara"]}`), `{"order":["I","am","Sara"]}`, true},
		{"reorder wrong", ex("reorder", `{"order":["I","am","Sara"]}`), `{"order":["am","I","Sara"]}`, false},
		{"match right (any order)", ex("match", `{"pairs":[["Hello","سلام"],["Bye","خداحافظ"]]}`), `{"pairs":[["Bye","خداحافظ"],["Hello","سلام"]]}`, true},
		{"match wrong", ex("match", `{"pairs":[["Hello","سلام"],["Bye","خداحافظ"]]}`), `{"pairs":[["Hello","خداحافظ"],["Bye","سلام"]]}`, false},
		{"select right (any order)", ex("select_words", `{"selected":[0,4]}`), `{"selected":[4,0]}`, true},
		{"select missing one", ex("select_words", `{"selected":[0,4]}`), `{"selected":[0]}`, false},
		{"select extra tile", ex("select_words", `{"selected":[0,4]}`), `{"selected":[0,4,2]}`, false},
		{"select wrong tile", ex("select_words", `{"selected":[0,4]}`), `{"selected":[0,3]}`, false},
		{"fill accepts an alternative", ex("fill_blank", `{"answers":["aren't","are not"]}`), `{"text":"are not"}`, true},
		// fill_gaps — ordered multi-blank, every gap must match (N01)
		{"gaps all correct", ex("fill_gaps", `{"gaps":[["'m","am"],["are"]]}`), `{"gaps":["am","Are."]}`, true},
		{"gaps second wrong", ex("fill_gaps", `{"gaps":[["'m","am"],["are"]]}`), `{"gaps":["'m","is"]}`, false},
		{"gaps swapped", ex("fill_gaps", `{"gaps":[["'m","am"],["are"]]}`), `{"gaps":["are","'m"]}`, false},
		{"gaps incomplete count", ex("fill_gaps", `{"gaps":[["'m"],["are"]]}`), `{"gaps":["'m"]}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, sol := grade(c.ex, json.RawMessage(c.answer))
			if got != c.want {
				t.Fatalf("grade=%v want=%v", got, c.want)
			}
			if len(sol) == 0 {
				t.Fatal("expected a solution for feedback")
			}
		})
	}
}

func TestGradeSpeak(t *testing.T) {
	e := ex(TypeSpeak, `{"min_score":70}`)
	if ok, _ := grade(e, json.RawMessage(`{"score":85}`)); !ok {
		t.Fatal("score 85 >= 70 should pass")
	}
	if ok, _ := grade(e, json.RawMessage(`{"score":60}`)); ok {
		t.Fatal("score 60 < 70 should fail")
	}
	if ok, _ := grade(ex(TypeSpeak, `{}`), json.RawMessage(`{"score":60}`)); !ok {
		t.Fatal("score 60 >= default 60 should pass")
	}
}
