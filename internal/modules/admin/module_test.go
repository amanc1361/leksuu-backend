package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leksuu/platform/internal/shared/authn"
)

type testGuard struct{ role string }

func (g testGuard) RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.role != role {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(authn.NewContext(context.Background(), authn.Principal{Subject: "operator-id", Roles: []string{"admin"}})))
	})
}

func TestLocalizationOnlyForAdminAndReplacesCredentials(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/catalog" || r.URL.Query().Get("locale") != "fa" {
			t.Errorf("wrong upstream route")
		}
		if r.Header.Get("Authorization") != "Bearer internal-token" || r.Header.Get("X-Localization-Actor") != "operator-id" {
			t.Errorf("wrong upstream identity")
		}
		if r.Header.Get("Cookie") != "" {
			t.Error("browser cookie leaked")
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	for _, role := range []string{"", "learner", "teacher", "admin"} {
		mux := http.NewServeMux()
		New(Deps{Guard: testGuard{role}, LocalizationURL: upstream.URL, LocalizationToken: "internal-token"}).Mount(mux, "/api/v1")
		req := httptest.NewRequest("GET", "/api/v1/admin/localization/catalog?locale=fa", nil)
		req.Header.Set("Authorization", "Bearer browser-jwt")
		req.Header.Set("X-Localization-Actor", "forged-actor")
		req.Header.Set("Cookie", "secret-cookie")
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		if role == "admin" {
			if out.Code != 200 || !strings.Contains(out.Body.String(), "true") {
				t.Errorf("admin: %d", out.Code)
			}
		} else if out.Code != 403 {
			t.Errorf("role %s reached workspace", role)
		}
	}
	if calls != 1 {
		t.Fatalf("upstream calls=%d", calls)
	}
}

func TestUnconfiguredLocalizationIsExplicitlyUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	New(Deps{Guard: testGuard{"admin"}}).Mount(mux, "/api/v1")
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, httptest.NewRequest("GET", "/api/v1/admin/localization/catalog", nil))
	if out.Code != 503 || !strings.Contains(out.Body.String(), "localization_not_configured") {
		t.Fatalf("unexpected response %d", out.Code)
	}
}
