package ai

import "testing"

func TestStripThink(t *testing.T) {
	cases := map[string]string{
		"<think>reasoning here</think>\nسلام":    "سلام",
		"<think>\nmulti\nline\n</think>  answer": "answer",
		"no think block":                         "no think block",
		"  spaced  ":                             "spaced",
		"<think>a</think><think>b</think>final":  "final",
	}
	for in, want := range cases {
		if got := stripThink(in); got != want {
			t.Errorf("stripThink(%q) = %q, want %q", in, got, want)
		}
	}
}
