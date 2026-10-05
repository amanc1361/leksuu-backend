package domain

import (
	"encoding/json"
	"testing"
)

func TestPhraseMeaningsAreLocalizable(t *testing.T) {
	payload := json.RawMessage(`{"phrase":"Hello!","translation":{"ckb":"سڵاو!"},"audio":"a.mp3"}`)
	strs := LocalizableStrings(KindText, payload)
	if len(strs) != 1 || strs[0].Kind != L10nPhrase || strs[0].Text != "Hello!" || strs[0].Lang != "en" {
		t.Fatalf("got %+v", strs)
	}
	tr := map[string]map[string]string{SourceHash("Hello!"): {"fa": "سلام!", "ckb": "stored"}}
	var p struct {
		Phrase      string            `json:"phrase"`
		Translation map[string]string `json:"translation"`
		Audio       string            `json:"audio"`
	}
	if err := json.Unmarshal(LocalizePayload(KindText, payload, tr), &p); err != nil {
		t.Fatal(err)
	}
	if p.Phrase != "Hello!" || p.Audio != "a.mp3" || p.Translation["fa"] != "سلام!" || p.Translation["ckb"] != "سڵاو!" {
		t.Fatalf("the stored fa meaning is added, the card's own ckb kept: %+v", p)
	}
	if got := LocalizePayload(KindText, json.RawMessage(`{"phrase":"Bye"}`), tr); string(got) != `{"phrase":"Bye"}` {
		t.Fatalf("a phrase without translations is untouched: %s", got)
	}
}
