package pronunciation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// stubScorer returns a deterministic, plausible assessment derived from the
// expected text — no model, no audio analysis. It lets the whole flow run and
// be tested before a real scoring service exists. NOT a real measurement.
type stubScorer struct{}

func (stubScorer) Name() string { return "stub" }

func (stubScorer) Score(_ context.Context, in domain.ScoreInput) (*domain.Assessment, error) {
	words := strings.Fields(in.Expected)
	out := make([]domain.WordScore, 0, len(words))
	sum := 0
	for _, w := range words {
		sc := stableScore(w)
		out = append(out, domain.WordScore{Word: w, Score: sc})
		sum += sc
	}
	overall := 80
	if len(out) > 0 {
		overall = sum / len(out)
	}
	return &domain.Assessment{Overall: overall, Transcript: in.Expected, Words: out}, nil
}

// stableScore maps a word to a deterministic value in 65..99.
func stableScore(w string) int {
	h := 0
	for _, c := range strings.ToLower(w) {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return 65 + h%35
}

// httpScorer POSTs the recording to an external scoring service (multipart:
// audio + expected + lang) and expects an Assessment JSON back.
type httpScorer struct {
	url    string
	apiKey string
	client *http.Client
}

func (p httpScorer) Name() string { return "http" }

func (p httpScorer) Score(ctx context.Context, in domain.ScoreInput) (*domain.Assessment, error) {
	if u, err := neturl.Parse(p.url); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, apperr.ErrBadRequest
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("expected", in.Expected)
	_ = mw.WriteField("lang", in.Lang)
	fw, err := mw.CreateFormFile("audio", "audio")
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(in.Audio); err != nil {
		return nil, err
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("pronunciation provider status %d", resp.StatusCode)
	}
	var a domain.Assessment
	var shape map[string]json.RawMessage
	if json.Unmarshal(data, &shape) != nil || shape["overall"] == nil || string(shape["overall"]) == "null" {
		return nil, apperr.New("pronunciation.bad_output", apperr.KindValidation)
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, apperr.New("pronunciation.bad_output", apperr.KindValidation)
	}
	return &a, nil
}
