package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	mediadomain "github.com/leksuu/platform/internal/modules/media/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

const CoverStyleVersion = "leksuu-painterly-v1"

var ErrCoverGeneratorUnavailable = errors.New("cover generator is not configured")
var ErrCoverGeneratorBusy = errors.New("cover generator is busy")
var coverPreviewKey = regexp.MustCompile(`^cover-previews/[a-f0-9-]+/[a-f0-9]{64}\.png$`)
var coverGenerationSlots = make(chan struct{}, 2)

type CoverPreview struct {
	URL    string `json:"url"`
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Style  string `json:"style"`
	Model  string `json:"model"`
}

type CoverGenerator struct {
	APIKey string
	Model  string
	Client *http.Client
}

func (g *CoverGenerator) Generate(ctx context.Context, prompt string) ([]byte, error) {
	if g == nil || g.APIKey == "" {
		return nil, ErrCoverGeneratorUnavailable
	}
	select {
	case coverGenerationSlots <- struct{}{}:
		defer func() { <-coverGenerationSlots }()
	default:
		return nil, ErrCoverGeneratorBusy
	}
	model := g.Model
	if model == "" {
		model = "gpt-image-2.5-flare"
	}
	requestBody, _ := json.Marshal(map[string]any{
		"model": model, "prompt": prompt, "n": 1, "size": "1024x1536",
		"quality": "medium", "output_format": "png", "background": "opaque",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/generations", bytes.NewReader(requestBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := g.Client
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image provider returned %d", resp.StatusCode)
	}
	var result struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 24<<20)).Decode(&result); err != nil || len(result.Data) == 0 {
		return nil, errors.New("image provider returned no image")
	}
	data, err := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
	if err != nil || len(data) == 0 || len(data) > 12<<20 {
		return nil, errors.New("image provider returned an invalid image")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "png" || config.Width < 512 || config.Height < 768 {
		return nil, errors.New("image provider returned an invalid cover")
	}
	return data, nil
}

func (s *Service) GenerateStoryCover(ctx context.Context, itemID, direction string, generator *CoverGenerator) (*CoverPreview, error) {
	item, err := s.findMediaItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if item.Kind != coursedomain.KindStory || s.store == nil {
		return nil, apperr.ErrBadRequest
	}
	var payload map[string]any
	if json.Unmarshal(item.Payload, &payload) != nil {
		return nil, apperr.ErrInternal
	}
	str := func(key string) string {
		switch value := payload[key].(type) {
		case string:
			return strings.TrimSpace(value)
		case []any:
			parts := make([]string, 0, len(value))
			for _, part := range value {
				if text, ok := part.(string); ok {
					parts = append(parts, text)
				}
			}
			return strings.Join(parts, ", ")
		}
		return ""
	}
	direction = strings.TrimSpace(direction)
	if len(direction) > 1200 || (direction == "" && str("description") == "" && str("genre") == "") {
		return nil, apperr.ErrBadRequest
	}
	prompt := strings.Join([]string{
		"Create a vertical 2:3 full-bleed original premium painterly editorial illustration for the Leksuu story library.",
		"Match the established Leksuu painterly cover direction: expressive brush texture, cinematic atmosphere, distinctive subject, calm upper area for application typography.",
		"Palette: violet #65458d, deep plum #403147, dusty lavender, muted sky blue and restrained coral. For children's stories, muted lime may be used.",
		"No text, letters, logos, watermark, frame or imitation of a commercial book cover. Do not invent plot details beyond the metadata and art direction.",
		"Title: " + str("title"), "Author: " + str("author"), "Genre: " + str("genre"),
		"Description: " + str("description"), "Audience: " + str("audience"),
		"Collection: " + str("collection"), "Series: " + str("series_id"),
		"Art direction from editor: " + direction,
	}, "\n")
	if len(prompt) > 30000 {
		return nil, apperr.ErrBadRequest
	}
	data, err := generator.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	key := "cover-previews/" + itemID + "/" + sha + ".png"
	url, err := s.store.Put(ctx, key, "image/png", data)
	if err != nil {
		return nil, err
	}
	model := generator.Model
	if model == "" {
		model = "gpt-image-2.5-flare"
	}
	return &CoverPreview{URL: url, Key: key, SHA256: sha, Style: CoverStyleVersion, Model: model}, nil
}

func (s *Service) ApproveStoryCover(ctx context.Context, itemID, key, expectedCover string) (string, error) {
	if !coverPreviewKey.MatchString(key) || !strings.HasPrefix(key, "cover-previews/"+itemID+"/") {
		return "", apperr.ErrBadRequest
	}
	lookup, ok := s.store.(mediadomain.Lookup)
	if !ok {
		return "", apperr.ErrInternal
	}
	if contentType, err := lookup.Stat(ctx, key); err != nil || contentType != "image/png" {
		return "", apperr.ErrBadRequest
	}
	item, err := s.findMediaItem(ctx, itemID)
	if err != nil {
		return "", err
	}
	if item.Kind != coursedomain.KindStory {
		return "", apperr.ErrBadRequest
	}
	var payload map[string]any
	if json.Unmarshal(item.Payload, &payload) != nil {
		return "", apperr.ErrInternal
	}
	current, _ := payload["cover"].(string)
	if current != expectedCover {
		return "", apperr.ErrBadRequest
	}
	url := lookup.URL(key)
	if current == url {
		return url, nil
	}
	if current != "" {
		versions, _ := payload["cover_versions"].([]any)
		payload["cover_versions"] = append(versions, current)
	}
	payload["cover"] = url
	payload["cover_key"] = key
	payload["cover_source"] = "generated"
	payload["cover_style"] = CoverStyleVersion
	payload["cover_asset"] = map[string]any{
		"key": key, "mime_type": "image/png", "sha256": strings.TrimSuffix(strings.TrimPrefix(key, "cover-previews/"+itemID+"/"), ".png"),
		"source": "generated", "style": CoverStyleVersion, "approved_at": time.Now().UTC().Format(time.RFC3339),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, itemID, item.Payload, encoded)
	if err != nil {
		return "", err
	}
	if !updated {
		return "", ErrMediaJobConflict
	}
	return url, nil
}

func (s *Service) RestorePreviousStoryCover(ctx context.Context, itemID, expectedCover string) (string, error) {
	item, err := s.findMediaItem(ctx, itemID)
	if err != nil {
		return "", err
	}
	if item.Kind != coursedomain.KindStory {
		return "", apperr.ErrBadRequest
	}
	var payload map[string]any
	if json.Unmarshal(item.Payload, &payload) != nil {
		return "", apperr.ErrInternal
	}
	current, _ := payload["cover"].(string)
	if current != expectedCover {
		return "", apperr.ErrBadRequest
	}
	versions, _ := payload["cover_versions"].([]any)
	if len(versions) == 0 {
		return "", apperr.ErrBadRequest
	}
	previous, ok := versions[len(versions)-1].(string)
	if !ok || previous == "" {
		return "", apperr.ErrBadRequest
	}
	payload["cover"] = previous
	payload["cover_versions"] = append(versions[:len(versions)-1], current)
	payload["cover_source"] = "restored"
	delete(payload, "cover_key")
	delete(payload, "cover_asset")
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, itemID, item.Payload, encoded)
	if err != nil {
		return "", err
	}
	if !updated {
		return "", ErrMediaJobConflict
	}
	return previous, nil
}
