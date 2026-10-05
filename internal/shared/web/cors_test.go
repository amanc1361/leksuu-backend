package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSAcceptsConfiguredOrigins(t *testing.T) {
	handler := CORS([]string{"https://leksuu.app", "http://localhost:5173"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, origin := range []string{"https://leksuu.app", "http://localhost:5173"} {
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("origin %q: got allow-origin %q", origin, got)
		}
		if rec.Code != http.StatusNoContent {
			t.Fatalf("origin %q: got status %d", origin, rec.Code)
		}
	}
}

func TestCORSRejectsUnconfiguredOrigin(t *testing.T) {
	handler := CORS([]string{"https://leksuu.app"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow-origin %q", got)
	}
}

func TestCORSWildcardAllowsAnyOriginWithoutCredentials(t *testing.T) {
	handler := CORS([]string{"*"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://frontend.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("got allow-origin %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("wildcard must not be combined with credentials, got %q", got)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got status %d", rec.Code)
	}
}

func TestCORSPreflightAllowsEveryMethodTheAPIUses(t *testing.T) {
	handler := CORS([]string{"https://leksuu.app"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://leksuu.app")
	req.Header.Set("Access-Control-Request-Method", http.MethodPut)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	methods := rec.Header().Get("Access-Control-Allow-Methods")
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		if !strings.Contains(methods, m) {
			t.Fatalf("preflight must allow %s, got %q", m, methods)
		}
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("a listed origin keeps credentials, got %q", got)
	}
}

func TestCORSListedOriginWinsOverWildcard(t *testing.T) {
	handler := CORS([]string{"*", "https://leksuu.app"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://leksuu.app")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://leksuu.app" {
		t.Fatalf("got allow-origin %q", got)
	}
}
