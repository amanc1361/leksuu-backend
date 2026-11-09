package usecase

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

func TestPodcastQuestionsPreserveSourceAndRejectStaleEditors(t *testing.T) {
	ctx := context.Background()
	repo := courserepo.NewMemoryCourseRepo()
	svc := NewService(repo, nil)
	start, end := 1.25, 4.75
	item, err := svc.ImportMedia(ctx, ImportMediaInput{Kind: "podcast", Title: "Fixture", Language: "en", URL: "/fixture.mp3",
		Transcript: []MediaCue{{Text: "We meet at the station.", Start: &start, End: &end, Translation: map[string]string{"fa": "fixture translation"}}}})
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]json.RawMessage
	_ = json.Unmarshal(item.Payload, &original)
	original["custom_metadata"] = json.RawMessage(`{"external_id":9007199254740993,"note":"preserve"}`)
	withMetadata, _ := json.Marshal(original)
	if changed, err := repo.UpdateItemPayloadIfUnchanged(ctx, item.ID, item.Payload, withMetadata); err != nil || !changed {
		t.Fatalf("fixture setup: %v", err)
	}
	item.Payload = withMetadata
	var before map[string]any
	_ = json.Unmarshal(item.Payload, &before)
	question := PodcastQuestion{ID: "where", CueIndex: 0, CueText: "We meet at the station.", Start: start, End: end,
		Prompt: "Where do they meet?", Options: []string{"At the station", "At home"}, CorrectIndex: 0, Explanation: "The speaker names the station.", ReviewStatus: "approved"}
	input := PodcastQuestionsInput{Questions: []PodcastQuestion{question}, ExpectedQuestions: json.RawMessage("null")}
	if err := svc.SavePodcastQuestions(ctx, item.ID, input); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.findMediaItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]any
	_ = json.Unmarshal(updated.Payload, &after)
	var savedRaw map[string]json.RawMessage
	_ = json.Unmarshal(updated.Payload, &savedRaw)
	if string(savedRaw["custom_metadata"]) != string(original["custom_metadata"]) {
		t.Fatal("question save changed unrelated metadata or large integer precision")
	}
	if len(after["listening_questions"].([]any)) != 1 {
		t.Fatal("missing saved question")
	}
	delete(after, "listening_questions")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("saving questions changed existing source fields")
	}
	if err := svc.SavePodcastQuestions(ctx, item.ID, input); err != ErrMediaJobConflict {
		t.Fatalf("stale save: %v", err)
	}
	expected, _ := json.Marshal(input.Questions)
	for name, mutate := range map[string]func(*PodcastQuestion){
		"changed text":      func(q *PodcastQuestion) { q.CueText = "Different source" },
		"changed timing":    func(q *PodcastQuestion) { q.Start++ },
		"invalid answer":    func(q *PodcastQuestion) { q.CorrectIndex = 99 },
		"duplicate options": func(q *PodcastQuestion) { q.Options = []string{"Same", " same "} },
		"unknown review":    func(q *PodcastQuestion) { q.ReviewStatus = "generated" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := question
			mutate(&invalid)
			if err := svc.SavePodcastQuestions(ctx, item.ID, PodcastQuestionsInput{Questions: []PodcastQuestion{invalid}, ExpectedQuestions: expected}); err == nil {
				t.Fatal("accepted invalid question")
			}
		})
	}
	unchanged, _ := svc.findMediaItem(ctx, item.ID)
	if string(unchanged.Payload) != string(updated.Payload) {
		t.Fatal("rejected saves changed stored data")
	}
}
