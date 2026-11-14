package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/repo"
	"github.com/leksuu/platform/internal/modules/auth/security"
	"github.com/leksuu/platform/internal/modules/auth/usecase"
)

func TestOnboardingRoutesRequireAccountAndKeepAnswersIsolated(t *testing.T) {
	jwt := security.NewJWT([]byte("onboarding-test-secret"), 15*time.Minute, time.Now)
	svc := usecase.NewService(repo.NewMemoryUserRepo(), repo.NewMemoryTokenRepo(),
		security.NewPasswordHasher(), jwt, security.NewRefreshFactory(), time.Now,
		usecase.Config{RefreshTTL: 24 * time.Hour}, nil)
	first, err := svc.Register(t.Context(), usecase.RegisterInput{Email: "first@example.com", Password: "password", DisplayName: "First"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Register(t.Context(), usecase.RegisterInput{Email: "second@example.com", Password: "password", DisplayName: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	auth := NewAuthenticator(jwt)
	auth.SetAccountChecker(svc)
	mux := http.NewServeMux()
	NewHandler(svc).Mount(mux, "/api/v1", auth)
	call := func(method, token, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1/me/onboarding", bytes.NewBufferString(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	firstToken, _, err := jwt.Issue(first.ID, []string{"student"})
	if err != nil {
		t.Fatal(err)
	}
	secondToken, _, err := jwt.Issue(second.ID, []string{"student"})
	if err != nil {
		t.Fatal(err)
	}
	if got := call(http.MethodGet, "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous GET: %d", got)
	}
	if got := call(http.MethodPut, "", `{"skipped":true}`).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous PUT: %d", got)
	}
	if got := call(http.MethodPut, firstToken, `{"answers":{"uiLang":"invalid"},"skipped":false}`).Code; got != http.StatusBadRequest {
		t.Fatalf("invalid PUT: %d", got)
	}
	if got := call(http.MethodPut, firstToken, `{"answers":{"uiLang":"ckb","target":"en","reason":"study","level":"A1","goal":10},"skipped":false}`).Code; got != http.StatusOK {
		t.Fatalf("valid PUT: %d", got)
	}
	var firstState, secondState struct {
		Completed bool `json:"completed"`
		Answers   struct {
			UILang string `json:"uiLang"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(call(http.MethodGet, firstToken, "").Body.Bytes(), &firstState); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(call(http.MethodGet, secondToken, "").Body.Bytes(), &secondState); err != nil {
		t.Fatal(err)
	}
	if !firstState.Completed || firstState.Answers.UILang != "ckb" {
		t.Fatalf("first state: %+v", firstState)
	}
	if secondState.Completed || secondState.Answers.UILang != "" {
		t.Fatalf("second state inherited answers: %+v", secondState)
	}
}
