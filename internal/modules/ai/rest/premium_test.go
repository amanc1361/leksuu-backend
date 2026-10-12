package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leksuu/platform/internal/modules/ai/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
)

// countingProvider records whether the model was called at all.
type countingProvider struct{ calls int }

func (p *countingProvider) Name() string { return "counting" }

func (p *countingProvider) Complete(_ context.Context, _, _ string) (string, error) {
	p.calls++
	return `{"reply":"Hallo!","translation":"Hello!"}`, nil
}

type premiumFor map[string]bool

func (p premiumFor) RequirePremium(_ context.Context, userID string) error {
	if p[userID] {
		return nil
	}
	return apperr.New("billing.premium_required", apperr.KindForbidden)
}

// The AI coach is not free: without the learning subscription the request is
// refused before any model call is made.
func TestCoachRequiresSubscription(t *testing.T) {
	prov := &countingProvider{}
	h := NewHandler(usecase.NewService(prov))
	h.SetPremium(premiumFor{"subscriber": true})
	call := func(user string) int {
		req := httptest.NewRequest(http.MethodPost, "/ai/chat", strings.NewReader(`{"messages":[{"role":"user","content":"Hallo"}],"target_lang":"de","native_lang":"fa","level":"A1"}`))
		req = req.WithContext(authn.NewContext(req.Context(), authn.Principal{Subject: user, Roles: []string{"student"}}))
		rec := httptest.NewRecorder()
		h.Chat(rec, req)
		return rec.Code
	}
	if code := call("free-user"); code != http.StatusForbidden || prov.calls != 0 {
		t.Fatalf("free user: code=%d model calls=%d (want 403, 0)", code, prov.calls)
	}
	if code := call("subscriber"); code == http.StatusForbidden || prov.calls == 0 {
		t.Fatalf("subscriber: code=%d model calls=%d (want through)", code, prov.calls)
	}
}
