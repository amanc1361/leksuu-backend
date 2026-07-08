package domain

import "context"

// Pair is two words that differ by exactly one sound — the heart of a minimal
// pair drill. Audio URLs are optional; the client falls back to speech synthesis.
type Pair struct {
	WordA  string `json:"word_a"`
	WordB  string `json:"word_b"`
	IpaA   string `json:"ipa_a"`
	IpaB   string `json:"ipa_b"`
	AudioA string `json:"audio_a,omitempty"`
	AudioB string `json:"audio_b,omitempty"`
}

// Contrast is a drillable sound opposition (e.g. /w/ vs /v/) chosen specifically
// for the mistakes Persian speakers make in English. Tip is the localized
// explanation of how to hear and produce each sound.
type Contrast struct {
	ID     string            `json:"id"`
	Label  string            `json:"label"`
	SoundA string            `json:"sound_a"`
	SoundB string            `json:"sound_b"`
	Tip    map[string]string `json:"tip"`
	Pairs  []Pair            `json:"pairs"`
}

type Repository interface {
	List(ctx context.Context) ([]Contrast, error)
	Get(ctx context.Context, id string) (*Contrast, error)
}
