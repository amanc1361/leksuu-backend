package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leksuu/platform/internal/modules/rtc/usecase"
	"github.com/leksuu/platform/internal/shared/authn"
)

type okParser struct{}

func (okParser) ParseToken(string) (authn.Principal, error) {
	return authn.Principal{Subject: "user-1"}, nil
}

type okAuthz struct{}

func (okAuthz) CanJoin(context.Context, string, string) (bool, string, error) {
	return true, "room-1", nil
}

type okIdent struct{}

func (okIdent) DisplayName(context.Context, string) string { return "Tester" }

// A ResponseWriter that cannot be hijacked models what happens behind a proxy
// that serves the backend over HTTP/2 (or otherwise wraps the writer): the
// WebSocket upgrade cannot complete.
func TestSignalReportsUpgradeFailureInsteadOfSilence(t *testing.T) {
	h := NewHandler(usecase.NewHub(), usecase.TurnSettings{}, okParser{}, okAuthz{}, okIdent{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rtc/signal?token=t&booking=b", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	rec := httptest.NewRecorder() // *httptest.ResponseRecorder is not an http.Hijacker

	h.Signal(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("upgrade failure returned 200 with an empty body; the client cannot tell what went wrong")
	}
	if !strings.Contains(rec.Body.String(), "rtc.upgrade_failed") {
		t.Fatalf("expected the rtc.upgrade_failed code in the body, got %q", rec.Body.String())
	}
}

// Missing Upgrade headers (a proxy that strips them) must also be reported.
func TestSignalReportsStrippedUpgradeHeaders(t *testing.T) {
	h := NewHandler(usecase.NewHub(), usecase.TurnSettings{}, okParser{}, okAuthz{}, okIdent{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rtc/signal?token=t&booking=b", nil)
	rec := httptest.NewRecorder()

	h.Signal(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("stripped upgrade headers returned 200 with an empty body")
	}
	if !strings.Contains(rec.Body.String(), "rtc.upgrade_failed") {
		t.Fatalf("expected rtc.upgrade_failed, got %q", rec.Body.String())
	}
}
