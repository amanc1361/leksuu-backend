package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/leksuu/platform/internal/shared/apperr"
)

// stubProvider returns deterministic content so the whole pipeline runs and is
// testable without any model server (dev / CI / before a GPU exists).
type stubProvider struct{}

func (stubProvider) Name() string { return "stub" }

func (stubProvider) Complete(_ context.Context, system, _ string) (string, error) {
	if strings.Contains(system, "writing coach") {
		return `{"corrected":"I go to the cafe. I drink coffee.","cefr":"A1","score":78,` +
			`"summary":"نوشته‌ی خوبی بود؛ چند خطای کوچک اصلاح شد.",` +
			`"issues":[{"original":"I goes","suggestion":"I go","explanation":"با I از شکلِ ساده‌ی فعل استفاده کن."}]}`, nil
	}
	if strings.Contains(system, "conversation tutor") {
		return `{"reply":"Nice! What food do you like?","translation":"عالی! چه غذایی دوست داری؟"}`, nil
	}
	if strings.Contains(system, "translator") {
		return "ترجمهٔ نمونه", nil
	}
	if strings.Contains(system, "JSON array") {
		return `[` +
			`{"lemma":"hello","meaning":"سلام","example":"Hello, how are you?"},` +
			`{"lemma":"thanks","meaning":"ممنون","example":"Thanks for your help."},` +
			`{"lemma":"water","meaning":"آب","example":"I drink water every day."}]`, nil
	}
	return `{"title":"A Day at the Cafe",` +
		`"story":"I go to a cafe. I drink coffee. The coffee is hot. I am happy.",` +
		`"story_translation":"به یک کافه می‌روم. قهوه می‌نوشم. قهوه داغ است. خوشحالم.",` +
		`"pov_questions":["Where do I go?","What do I drink?"],` +
		`"exercises":[` +
		`{"kind":"mcq","prompt":"What do I drink?","options":["coffee","tea","water"],"answer":"coffee"},` +
		`{"kind":"true_false","prompt":"The coffee is cold.","answer":"false"},` +
		`{"kind":"fill_blank","prompt":"I drink ___.","answer":"coffee"}]}`, nil
}

// openAIProvider calls any OpenAI-compatible chat endpoint (vLLM, Ollama, etc.).
// baseURL should point at the API root that exposes /chat/completions.
type openAIProvider struct {
	baseURL string
	model   string
	apiKey  string
	noThink bool // append /no_think so reasoning models (Qwen3) return the answer directly
	client  *http.Client
}

// thinkRe matches a <think>...</think> reasoning block some models inline.
var thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// stripThink removes any inlined reasoning block and trims the rest.
func stripThink(s string) string {
	return strings.TrimSpace(thinkRe.ReplaceAllString(s, ""))
}

func (p openAIProvider) Name() string { return "openai:" + p.model }

func (p openAIProvider) Complete(ctx context.Context, system, user string) (string, error) {
	url := strings.TrimRight(p.baseURL, "/") + "/chat/completions"
	if u, err := neturl.Parse(url); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", apperr.ErrBadRequest
	}
	u := user
	if p.noThink {
		// Qwen3 (and other Ollama "thinking" models) disable reasoning on this hint;
		// non-thinking models simply ignore it.
		u = user + "\n\n/no_think"
	}
	payload := map[string]any{
		"model":       p.model,
		"temperature": 0.4,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": u},
		},
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ai provider status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &out) != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("ai provider: unexpected response")
	}
	return stripThink(out.Choices[0].Message.Content), nil
}
