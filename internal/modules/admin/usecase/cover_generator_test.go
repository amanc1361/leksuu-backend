package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
	mediadomain "github.com/leksuu/platform/internal/modules/media/domain"
)

type coverTransport func(*http.Request) (*http.Response, error)
func (f coverTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type coverStoryStore struct { storyStore }
func (s *coverStoryStore) URL(key string) string { return "https://cdn.example/" + key }
func (s *coverStoryStore) Stat(_ context.Context, key string) (string, error) {
	if _, ok := s.objects[key]; !ok { return "", mediadomain.ErrNotFound }
	return "image/png", nil
}

func TestStoryCoverPreviewApprovalAndRestore(t *testing.T) {
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 512, 768))); err != nil { t.Fatal(err) }
	encodedImage := base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	provider := &CoverGenerator{APIKey: "test-only", Model: "gpt-image-2.5-flare", Client: &http.Client{Transport: coverTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://api.openai.com/v1/images/generations" || req.Header.Get("Authorization") != "Bearer test-only" {
			t.Fatal("provider request was not sent securely")
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil { t.Fatal(err) }
		if body["model"] != "gpt-image-2.5-flare" || !strings.Contains(body["prompt"].(string), "Editor approved setting") {
			t.Fatalf("provider request lost model or art direction: %+v", body)
		}
		response, _ := json.Marshal(map[string]any{"data": []map[string]string{{"b64_json": encodedImage}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(response)), Header: http.Header{}}, nil
	})}}
	ctx := context.Background()
	store := &coverStoryStore{}
	svc := NewService(courserepo.NewMemoryCourseRepo(), nil, store)
	archive, err := os.ReadFile("testdata/leksuu-story-import-test-valid.zip")
	if err != nil { t.Fatal(err) }
	if _, err := svc.ImportStoryArchive(ctx, archive, false); err != nil { t.Fatal(err) }
	items, err := svc.AdminMedia(ctx, "story")
	if err != nil || len(items) != 1 { t.Fatalf("story import: %v %d", err, len(items)) }
	id := items[0].ID
	var payload map[string]any
	if err := json.Unmarshal(items[0].Payload, &payload); err != nil { t.Fatal(err) }
	payload["cover"] = "https://cdn.example/original.webp"
	body, _ := json.Marshal(payload)
	if _, err := svc.UpdateItem(ctx, id, body, items[0].MediaKey); err != nil { t.Fatal(err) }
	preview, err := svc.GenerateStoryCover(ctx, id, "Editor approved setting", provider)
	if err != nil { t.Fatal(err) }
	if preview.Style != CoverStyleVersion || preview.SHA256 == "" || preview.URL == "" { t.Fatalf("invalid preview: %+v", preview) }
	before, err := svc.findMediaItem(ctx, id)
	if err != nil { t.Fatal(err) }
	if !bytes.Contains(before.Payload, []byte("original.webp")) || bytes.Contains(before.Payload, []byte(preview.Key)) {
		t.Fatal("preview changed approved story cover")
	}
	approved, err := svc.ApproveStoryCover(ctx, id, preview.Key, "https://cdn.example/original.webp")
	if err != nil || approved != preview.URL { t.Fatalf("approval: %v %s", err, approved) }
	if _, err := svc.ApproveStoryCover(ctx, id, preview.Key, "https://cdn.example/original.webp"); err == nil {
		t.Fatal("stale cover approval was accepted")
	}
	restored, err := svc.RestorePreviousStoryCover(ctx, id, approved)
	if err != nil || restored != "https://cdn.example/original.webp" { t.Fatalf("restore: %v %s", err, restored) }
}
