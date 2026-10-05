package rest

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicIPBlocksInternalRanges(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "0.0.0.0", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "64:ff9b::7f00:1", "198.18.0.1"} {
		if publicIP(net.ParseIP(s)) {
			t.Errorf("%s must be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicIP(net.ParseIP(s)) {
			t.Errorf("%s must be allowed", s)
		}
	}
}

// The dial-time guard refuses loopback even when the name check was passed
// (DNS rebinding) — e.g. the API's own endpoints.
func TestGuardedDialerRefusesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	if _, err := guardedDialer.DialContext(context.Background(), "tcp", srv.Listener.Addr().String()); err == nil {
		t.Fatal("dialing loopback must fail")
	}
}

func TestMediaIsServedInert(t *testing.T) {
	rec := httptest.NewRecorder()
	inert(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/x", nil))
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Fatalf("headers: %v", rec.Header())
	}
}
