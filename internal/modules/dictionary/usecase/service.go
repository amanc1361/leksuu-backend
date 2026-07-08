// Package usecase holds dictionary-provider rules: CRUD over providers and the
// word→meaning lookup that calls them and extracts the meaning from the response.
package usecase

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/leksuu/platform/internal/modules/dictionary/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Fetcher performs the outbound HTTP request to a provider. Injected so the call
// surface stays testable and the network policy lives in one place.
type Fetcher interface {
	Fetch(ctx context.Context, method, rawURL string, headers map[string]string, body string) (status int, data []byte, err error)
}

type Service struct {
	repo  domain.Repository
	fetch Fetcher
}

func NewService(repo domain.Repository, fetch Fetcher) *Service {
	return &Service{repo: repo, fetch: fetch}
}

func (s *Service) List(ctx context.Context) ([]domain.Provider, error) { return s.repo.List(ctx) }

func (s *Service) Create(ctx context.Context, p *domain.Provider) error {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.URL) == "" {
		return apperr.ErrBadRequest
	}
	if p.Method == "" {
		p.Method = "GET"
	}
	return s.repo.Create(ctx, p)
}

func (s *Service) Update(ctx context.Context, p *domain.Provider) error {
	if p.ID == "" {
		return apperr.ErrBadRequest
	}
	if p.Method == "" {
		p.Method = "GET"
	}
	return s.repo.Update(ctx, p)
}

func (s *Service) Delete(ctx context.Context, id string) error { return s.repo.Delete(ctx, id) }

// LookupResult is one provider's answer for a word.
type LookupResult struct {
	Word     string `json:"word"`
	Meaning  string `json:"meaning"`
	Provider string `json:"provider"`
}

// Lookup tries each enabled provider in order and returns the first non-empty
// meaning. The provider that answered is reported back for transparency.
func (s *Service) Lookup(ctx context.Context, word string) (*LookupResult, error) {
	word = strings.TrimSpace(word)
	if word == "" {
		return nil, apperr.ErrBadRequest
	}
	provs, err := s.repo.Enabled(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range provs {
		u := strings.ReplaceAll(p.URL, "{word}", url.QueryEscape(word))
		method := p.Method
		if method == "" {
			method = "GET"
		}
		headers := map[string]string{}
		if p.HeaderName != "" {
			headers[p.HeaderName] = p.HeaderValue
		}
		body := strings.ReplaceAll(p.BodyTemplate, "{word}", word)
		status, data, err := s.fetch.Fetch(ctx, method, u, headers, body)
		if err != nil || status >= 400 {
			continue
		}
		if m := extractPath(data, p.ResponsePath); m != "" {
			return &LookupResult{Word: word, Meaning: m, Provider: p.Name}, nil
		}
	}
	return nil, domain.ErrNoResult
}

// extractPath walks a dot-separated path into a decoded JSON value. Numeric
// segments index arrays; everything else is a map key. Returns "" on any miss.
func extractPath(data []byte, path string) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	if strings.TrimSpace(path) != "" {
		for _, seg := range strings.Split(path, ".") {
			switch cur := v.(type) {
			case map[string]any:
				v = cur[seg]
			case []any:
				idx, err := strconv.Atoi(seg)
				if err != nil || idx < 0 || idx >= len(cur) {
					return ""
				}
				v = cur[idx]
			default:
				return ""
			}
			if v == nil {
				return ""
			}
		}
	}
	return stringify(v)
}

func stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
