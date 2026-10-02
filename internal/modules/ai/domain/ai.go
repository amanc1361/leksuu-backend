// Package domain holds the AI-authoring types: a structured content draft the
// model proposes, which an admin reviews before any of it is published.
package domain

// DraftInput is what the admin asks the model to draft.
type DraftInput struct {
	Topic      string
	Level      string // CEFR, e.g. A1
	TargetLang string // language being learned
	NativeLang string // explanation language
}

// DraftExercise is one proposed exercise. Kind is mcq | fill_blank | true_false.
type DraftExercise struct {
	Kind    string   `json:"kind"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options,omitempty"`
	Answer  string   `json:"answer"`
}

// Draft is the model's proposed lesson content: a POV mini-story plus exercises.
type Draft struct {
	Title            string          `json:"title"`
	Story            string          `json:"story"`
	StoryTranslation string          `json:"story_translation"`
	PovQuestions     []string        `json:"pov_questions"`
	Exercises        []DraftExercise `json:"exercises"`
}

// VocabInput asks the model for a batch of vocabulary in a target language.
type VocabInput struct {
	TargetLang string
	NativeLang string
	Level      string
	Topic      string
	Count      int
}

// DraftVocab is one proposed vocabulary item; meaning is in the native language.
type DraftVocab struct {
	Lemma   string `json:"lemma"`
	Meaning string `json:"meaning"`
	Example string `json:"example"`
}

// TranslateInput asks the model to translate a word/phrase between languages.
type TranslateInput struct {
	Text     string
	FromLang string
	ToLang   string
}

// --- Learner-facing AI (Phase 3): writing coach + level-limited chat ---

// WritingInput is a learner's text submitted for correction + CEFR assessment.
type WritingInput struct {
	Text       string
	TargetLang string
	NativeLang string
	Level      string
}

// WritingIssue is one correction with a native-language explanation.
type WritingIssue struct {
	Original    string `json:"original"`
	Suggestion  string `json:"suggestion"`
	Explanation string `json:"explanation"`
}

// WritingFeedback is the coach's assessment of a learner's writing.
type WritingFeedback struct {
	Corrected string         `json:"corrected"`
	CEFR      string         `json:"cefr"`
	Score     int            `json:"score"` // 0-100
	Summary   string         `json:"summary"`
	Issues    []WritingIssue `json:"issues"`
}

// ChatMessage is one turn of a conversation. Role is "user" or "assistant".
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatScenario turns the chat into a roleplay: the tutor plays AIRole in the
// described situation and steers the learner toward Goal.
type ChatScenario struct {
	Title       string `json:"title"`
	Goal        string `json:"goal"`
	AIRole      string `json:"ai_role"`
	LearnerRole string `json:"learner_role"`
}

// ChatInput is a level-limited conversation request. With a Scenario and no
// messages yet, the tutor opens the roleplay.
type ChatInput struct {
	Messages   []ChatMessage
	TargetLang string
	NativeLang string
	Level      string
	Scenario   *ChatScenario
}

// ChatReply is the tutor's response: a target-language reply plus a translation.
type ChatReply struct {
	Reply       string `json:"reply"`
	Translation string `json:"translation"`
}
