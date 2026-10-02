// Package usecase turns a draft request into a prompt, calls the AI provider,
// and parses the model's JSON back into a Draft. The provider is a port so the
// same code runs against a stub, a self-hosted open model, or any OpenAI-
// compatible endpoint.
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leksuu/platform/internal/modules/ai/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Provider performs a single chat-style completion. Name identifies the backend.
type Provider interface {
	Name() string
	Complete(ctx context.Context, system, user string) (string, error)
}

type Service struct{ provider Provider }

func NewService(p Provider) *Service { return &Service{provider: p} }

func (s *Service) ProviderName() string { return s.provider.Name() }

const systemPrompt = `You are a language-learning content author. ` +
	`Produce A1-friendly content for a learner. ` +
	`Respond with STRICT JSON only — no markdown, no commentary. ` +
	`Schema: {"title":string,"story":string,"story_translation":string,` +
	`"pov_questions":[string],"exercises":[{"kind":"mcq|fill_blank|true_false","prompt":string,"options":[string],"answer":string}]}.`

const systemPromptVocab = `You are a vocabulary author. Produce CEFR-appropriate vocabulary. ` +
	`Respond with a STRICT JSON array only — no markdown, no commentary. ` +
	`Schema: [{"lemma":string,"meaning":string,"example":string}].`

const systemPromptTranslate = `You are a precise bilingual translator. ` +
	`Respond with ONLY the translation of the given word or phrase — ` +
	`no quotes, no romanization, no notes, no extra text.`

// Translate returns the meaning of a word/phrase in the target language.
func (s *Service) Translate(ctx context.Context, in domain.TranslateInput) (string, error) {
	if strings.TrimSpace(in.Text) == "" {
		return "", apperr.ErrBadRequest
	}
	if in.ToLang == "" {
		in.ToLang = "fa"
	}
	user := fmt.Sprintf("Translate the word or phrase %q from %s to %s. Respond with only the translation.",
		in.Text, in.FromLang, in.ToLang)
	raw, err := s.provider.Complete(ctx, systemPromptTranslate, user)
	if err != nil {
		return "", err
	}
	out := cleanTranslation(raw)
	if out == "" {
		return "", apperr.New("ai.bad_output", apperr.KindValidation)
	}
	return out, nil
}

// cleanTranslation strips fences/quotes and keeps the first non-empty line.
func cleanTranslation(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "\"'“”«»")
		if line != "" {
			return line
		}
	}
	return ""
}

func (s *Service) Draft(ctx context.Context, in domain.DraftInput) (*domain.Draft, error) {
	if strings.TrimSpace(in.Topic) == "" {
		return nil, apperr.ErrBadRequest
	}
	if in.Level == "" {
		in.Level = "A1"
	}
	if in.TargetLang == "" {
		in.TargetLang = "en"
	}
	user := fmt.Sprintf(
		"Write a short POV mini-story (3-6 simple sentences) in %q at CEFR %s about: %q. "+
			"Provide a translation of the story into %q in story_translation. "+
			"Add 2-3 point-of-view comprehension questions in pov_questions, and 2-3 exercises. "+
			"All story text must be in %q.",
		in.TargetLang, in.Level, in.Topic, in.NativeLang, in.TargetLang)

	raw, err := s.provider.Complete(ctx, systemPrompt, user)
	if err != nil {
		return nil, err
	}
	d, err := parseDraft(raw)
	if err != nil {
		return nil, apperr.New("ai.bad_output", apperr.KindValidation)
	}
	return d, nil
}

// DraftVocab asks the model for a batch of vocabulary items in the target
// language with meanings in the native language.
func (s *Service) DraftVocab(ctx context.Context, in domain.VocabInput) ([]domain.DraftVocab, error) {
	if in.TargetLang == "" {
		in.TargetLang = "en"
	}
	if in.Level == "" {
		in.Level = "A1"
	}
	n := in.Count
	if n <= 0 || n > 30 {
		n = 10
	}
	user := fmt.Sprintf(
		"List %d CEFR %s vocabulary items in %q about %q. "+
			"For each item give: lemma in %q, meaning in %q, and one short example sentence in %q. "+
			"Keep it simple and useful for a beginner.",
		n, in.Level, in.TargetLang, in.Topic, in.TargetLang, in.NativeLang, in.TargetLang)

	raw, err := s.provider.Complete(ctx, systemPromptVocab, user)
	if err != nil {
		return nil, err
	}
	items, err := parseVocabArray(raw)
	if err != nil {
		return nil, apperr.New("ai.bad_output", apperr.KindValidation)
	}
	return items, nil
}

func parseVocabArray(raw string) ([]domain.DraftVocab, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if i := strings.IndexByte(s, '['); i >= 0 {
		if j := strings.LastIndexByte(s, ']'); j >= i {
			s = s[i : j+1]
		}
	}
	var items []domain.DraftVocab
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("empty vocab")
	}
	return items, nil
}

// parseDraft extracts the JSON object from the model output (tolerating code
// fences or surrounding prose) and unmarshals it.
func parseDraft(raw string) (*domain.Draft, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if i := strings.IndexByte(s, '{'); i >= 0 {
		if j := strings.LastIndexByte(s, '}'); j >= i {
			s = s[i : j+1]
		}
	}
	var d domain.Draft
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		return nil, err
	}
	if strings.TrimSpace(d.Story) == "" {
		return nil, fmt.Errorf("empty story")
	}
	return &d, nil
}

// --- Learner-facing AI (Phase 3) ---

const systemPromptWriting = `You are a friendly writing coach for a language learner. ` +
	`Assess and correct the learner's writing. ` +
	`Respond with STRICT JSON only — no markdown, no commentary. ` +
	`Schema: {"corrected":string,"cefr":string,"score":number,"summary":string,` +
	`"issues":[{"original":string,"suggestion":string,"explanation":string}]}. ` +
	`"score" is an integer 0-100. "cefr" is one of A1,A2,B1,B2,C1,C2. ` +
	`Write "summary" and every "explanation" in the learner's native language. ` +
	`Stay faithful to what the learner meant; do not invent new content.`

// WritingFeedback corrects a learner's text and estimates its CEFR level.
func (s *Service) WritingFeedback(ctx context.Context, in domain.WritingInput) (*domain.WritingFeedback, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, apperr.ErrBadRequest
	}
	if in.TargetLang == "" {
		in.TargetLang = "en"
	}
	if in.NativeLang == "" {
		in.NativeLang = "fa"
	}
	if in.Level == "" {
		in.Level = "A1"
	}
	user := fmt.Sprintf(
		"The learner studies %q at CEFR %s; their native language is %q. "+
			"Correct and assess this text:\n%q",
		in.TargetLang, in.Level, in.NativeLang, in.Text)
	raw, err := s.provider.Complete(ctx, systemPromptWriting, user)
	if err != nil {
		return nil, err
	}
	fb, err := parseWriting(raw)
	if err != nil {
		return nil, apperr.New("ai.bad_output", apperr.KindValidation)
	}
	return fb, nil
}

const systemPromptChat = `You are a patient, friendly conversation tutor for a language learner. ` +
	`Always reply in the TARGET language using only simple words and short sentences ` +
	`suited to the learner's CEFR level, and ask one short follow-up question. ` +
	`Respond with STRICT JSON only — no markdown, no commentary. ` +
	`Schema: {"reply":string,"translation":string}. ` +
	`"reply" is in the target language; "translation" is that reply in the native language.`

// Chat continues a level-limited conversation as a tutor.
func (s *Service) Chat(ctx context.Context, in domain.ChatInput) (*domain.ChatReply, error) {
	sc := cleanScenario(in.Scenario)
	if len(in.Messages) == 0 && sc == nil {
		return nil, apperr.ErrBadRequest
	}
	if len(in.Messages) > maxChatTurns {
		in.Messages = in.Messages[len(in.Messages)-maxChatTurns:]
	}
	if in.TargetLang == "" {
		in.TargetLang = "en"
	}
	if in.NativeLang == "" {
		in.NativeLang = "fa"
	}
	if in.Level == "" {
		in.Level = "A1"
	}
	var b strings.Builder
	for _, m := range in.Messages {
		role := "Learner"
		if m.Role == "assistant" {
			role = "Tutor"
		}
		fmt.Fprintf(&b, "%s: %s\n", role, strings.TrimSpace(m.Content))
	}
	user := fmt.Sprintf(
		"Target language: %s. Native language: %s. CEFR level: %s.\n"+
			"Continue as the Tutor. Conversation so far:\n%s",
		in.TargetLang, in.NativeLang, in.Level, b.String())
	if sc != nil {
		role := sc.AIRole
		if role == "" {
			role = "a friendly colleague"
		}
		user = fmt.Sprintf(
			"Target language: %s. Native language: %s. CEFR level: %s.\n"+
				"ROLEPLAY. Situation: %s. You play %s; the learner plays %s. "+
				"The learner's goal: %s. Stay in character, keep every turn to one or two short sentences, "+
				"and steer the conversation so the learner gets to practise that goal. "+
				"If the learner makes a mistake, model the correct form naturally in your reply.\n",
			in.TargetLang, in.NativeLang, in.Level, sc.Title, role, orDefault(sc.LearnerRole, "themself"), orDefault(sc.Goal, sc.Title))
		if len(in.Messages) == 0 {
			user += "Open the conversation with your first line."
		} else {
			user += "Conversation so far:\n" + b.String()
		}
	}
	raw, err := s.provider.Complete(ctx, systemPromptChat, user)
	if err != nil {
		return nil, err
	}
	rep, err := parseChat(raw)
	if err != nil {
		return nil, apperr.New("ai.bad_output", apperr.KindValidation)
	}
	return rep, nil
}

// maxChatTurns keeps the prompt bounded on long conversations; the tutor only
// needs the recent turns to stay coherent.
const maxChatTurns = 24

// cleanScenario trims a roleplay scenario and caps its fields (they come from
// lesson content, but still reach the model verbatim). Nil when it is empty.
func cleanScenario(sc *domain.ChatScenario) *domain.ChatScenario {
	if sc == nil {
		return nil
	}
	clip := func(s string) string {
		s = strings.TrimSpace(s)
		if r := []rune(s); len(r) > 200 {
			s = string(r[:200])
		}
		return s
	}
	out := &domain.ChatScenario{Title: clip(sc.Title), Goal: clip(sc.Goal), AIRole: clip(sc.AIRole), LearnerRole: clip(sc.LearnerRole)}
	if out.Title == "" && out.Goal == "" {
		return nil
	}
	if out.Title == "" {
		out.Title = out.Goal
	}
	return out
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// extractJSONObject pulls the outermost {...} from model output, tolerating
// code fences or surrounding prose.
func extractJSONObject(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if i := strings.IndexByte(s, '{'); i >= 0 {
		if j := strings.LastIndexByte(s, '}'); j >= i {
			s = s[i : j+1]
		}
	}
	return s
}

func parseWriting(raw string) (*domain.WritingFeedback, error) {
	var fb domain.WritingFeedback
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &fb); err != nil {
		return nil, err
	}
	if strings.TrimSpace(fb.Corrected) == "" {
		return nil, fmt.Errorf("empty corrected")
	}
	if fb.CEFR == "" {
		fb.CEFR = "A1"
	}
	if fb.Score < 0 {
		fb.Score = 0
	}
	if fb.Score > 100 {
		fb.Score = 100
	}
	return &fb, nil
}

func parseChat(raw string) (*domain.ChatReply, error) {
	var rep domain.ChatReply
	if err := json.Unmarshal([]byte(extractJSONObject(raw)), &rep); err != nil {
		return nil, err
	}
	if strings.TrimSpace(rep.Reply) == "" {
		return nil, fmt.Errorf("empty reply")
	}
	return &rep, nil
}
