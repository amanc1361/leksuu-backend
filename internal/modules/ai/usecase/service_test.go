package usecase

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/ai/domain"
)

type fakeProvider struct{ out string }

func (fakeProvider) Name() string                                               { return "fake" }
func (f fakeProvider) Complete(context.Context, string, string) (string, error) { return f.out, nil }

func TestDraftParsesFencedJSON(t *testing.T) {
	svc := NewService(fakeProvider{out: "```json\n{\"title\":\"T\",\"story\":\"I go. I eat.\",\"story_translation\":\"x\",\"pov_questions\":[\"q1\"],\"exercises\":[{\"kind\":\"mcq\",\"prompt\":\"p\",\"options\":[\"a\",\"b\"],\"answer\":\"a\"}]}\n```"})
	d, err := svc.Draft(context.Background(), domain.DraftInput{Topic: "cafe"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if d.Story == "" || len(d.Exercises) != 1 || d.Exercises[0].Kind != "mcq" {
		t.Fatalf("bad parse: %+v", d)
	}
}

func TestDraftRejectsEmptyTopic(t *testing.T) {
	svc := NewService(fakeProvider{out: "{}"})
	if _, err := svc.Draft(context.Background(), domain.DraftInput{}); err == nil {
		t.Fatal("expected error on empty topic")
	}
}

func TestDraftRejectsGarbage(t *testing.T) {
	svc := NewService(fakeProvider{out: "not json at all"})
	if _, err := svc.Draft(context.Background(), domain.DraftInput{Topic: "x"}); err == nil {
		t.Fatal("expected error on non-JSON output")
	}
}

func TestDraftVocabParsesArray(t *testing.T) {
	svc := NewService(fakeProvider{out: "```json\n[{\"lemma\":\"hello\",\"meaning\":\"سلام\",\"example\":\"Hello!\"},{\"lemma\":\"water\",\"meaning\":\"آب\",\"example\":\"Water is good.\"}]\n```"})
	items, err := svc.DraftVocab(context.Background(), domain.VocabInput{TargetLang: "en", NativeLang: "fa", Topic: "basics", Count: 2})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(items) != 2 || items[0].Lemma != "hello" || items[1].Meaning != "آب" {
		t.Fatalf("bad parse: %+v", items)
	}
}

func TestDraftVocabRejectsObject(t *testing.T) {
	svc := NewService(fakeProvider{out: "{\"not\":\"an array\"}"})
	if _, err := svc.DraftVocab(context.Background(), domain.VocabInput{TargetLang: "en", Topic: "x"}); err == nil {
		t.Fatal("expected error when output is not a JSON array")
	}
}

func TestTranslateCleansOutput(t *testing.T) {
	svc := NewService(fakeProvider{out: "  \"Hallo\"\n"})
	out, err := svc.Translate(context.Background(), domain.TranslateInput{Text: "hello", FromLang: "en", ToLang: "de"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out != "Hallo" {
		t.Fatalf("want cleaned 'Hallo', got %q", out)
	}
}

func TestTranslateRejectsEmpty(t *testing.T) {
	svc := NewService(fakeProvider{out: "x"})
	if _, err := svc.Translate(context.Background(), domain.TranslateInput{Text: "  "}); err == nil {
		t.Fatal("expected error on empty text")
	}
}

// --- Learner-facing AI (Phase 3): writing coach + chat ---

func TestWritingFeedbackParses(t *testing.T) {
	svc := NewService(fakeProvider{out: "```json\n{\"corrected\":\"I go home.\",\"cefr\":\"A1\",\"score\":80," +
		"\"summary\":\"خوب بود\",\"issues\":[{\"original\":\"I goes\",\"suggestion\":\"I go\",\"explanation\":\"با I فعل ساده می‌آید\"}]}\n```"})
	fb, err := svc.WritingFeedback(context.Background(), domain.WritingInput{Text: "I goes home"})
	if err != nil {
		t.Fatal(err)
	}
	if fb.Corrected == "" || fb.CEFR != "A1" || fb.Score != 80 || len(fb.Issues) != 1 {
		t.Fatalf("unexpected feedback: %+v", fb)
	}
}

func TestWritingFeedbackRejectsEmptyText(t *testing.T) {
	svc := NewService(fakeProvider{out: "{}"})
	if _, err := svc.WritingFeedback(context.Background(), domain.WritingInput{Text: "   "}); err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestWritingFeedbackClampsScore(t *testing.T) {
	svc := NewService(fakeProvider{out: "{\"corrected\":\"x\",\"cefr\":\"A2\",\"score\":150,\"summary\":\"s\",\"issues\":[]}"})
	fb, err := svc.WritingFeedback(context.Background(), domain.WritingInput{Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if fb.Score != 100 {
		t.Fatalf("want score clamped to 100, got %d", fb.Score)
	}
}

func TestChatParses(t *testing.T) {
	svc := NewService(fakeProvider{out: "{\"reply\":\"Hi! How are you?\",\"translation\":\"سلام! حالت چطوره؟\"}"})
	rep, err := svc.Chat(context.Background(), domain.ChatInput{Messages: []domain.ChatMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Reply == "" || rep.Translation == "" {
		t.Fatalf("unexpected reply: %+v", rep)
	}
}

func TestChatRejectsEmptyHistory(t *testing.T) {
	svc := NewService(fakeProvider{out: "{}"})
	if _, err := svc.Chat(context.Background(), domain.ChatInput{}); err == nil {
		t.Fatal("expected error for empty messages")
	}
}
