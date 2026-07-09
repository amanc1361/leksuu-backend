package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAcceptsConfiguredOrigins(t *testing.T) {
	handler := CORS("https://leksuu.app, http://localhost:5173", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
	handler := CORS("https://leksuu.app", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow-origin %q", got)
	}
}

func TestCORSWildcardAllowsAnyOriginWithoutCredentials(t *testing.T) {
	handler := CORS("*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
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
