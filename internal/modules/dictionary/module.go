// Package dictionary is the module facade for external dictionary providers:
// admin-configured APIs that turn a word into a meaning. The lookup runs
// server-side. NOTE: providers call admin-supplied URLs; restrict to trusted
// admins (SSRF surface) and add host allow-listing before exposing widely.
package dictionary

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/dictionary/domain"
	"github.com/leksuu/platform/internal/modules/dictionary/rest"
	"github.com/leksuu/platform/internal/modules/dictionary/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
)

type Deps struct {
	Repo  domain.Repository
	Guard rest.Guard
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	return &Module{handler: rest.NewHandler(usecase.NewService(d.Repo, httpFetcher{client: &http.Client{Timeout: 8 * time.Second}})), guard: d.Guard}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }

// httpFetcher is the default outbound caller. Allows only http/https.
type httpFetcher struct{ client *http.Client }

func (f httpFetcher) Fetch(ctx context.Context, method, rawURL string, headers map[string]string, body string) (int, []byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return 0, nil, apperr.ErrBadRequest
	}
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, nil
}
