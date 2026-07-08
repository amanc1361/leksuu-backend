package usecase

import (
	"strings"
	"testing"
	"time"
)

func TestICEConfigStunOnlyWhenNoSecret(t *testing.T) {
	cfg := ICEConfigFor("u1", TurnSettings{StunURL: "stun:turn.x:3478"}, time.Now())
	if len(cfg.ICEServers) != 1 || cfg.ICEServers[0].Username != "" {
		t.Fatalf("expected stun-only without creds: %+v", cfg.ICEServers)
	}
}

func TestICEConfigEphemeralTurnCreds(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cfg := ICEConfigFor("user-42", TurnSettings{
		StunURL: "stun:turn.x:3478", TurnURL: "turn:turn.x:3478",
		TurnTLSURL: "turns:turn.x:5349", Secret: "s3cr3t", TTL: 3600,
	}, now)
	if len(cfg.ICEServers) != 2 {
		t.Fatalf("want stun+turn servers, got %d", len(cfg.ICEServers))
	}
	turn := cfg.ICEServers[1]
	// username = "<expiry>:<userID>", expiry = now+TTL
	wantUser := "1003600:user-42"
	if turn.Username != wantUser {
		t.Fatalf("username = %q want %q", turn.Username, wantUser)
	}
	if turn.Credential == "" {
		t.Fatal("expected HMAC credential")
	}
	if !strings.Contains(strings.Join(turn.URLs, ","), "turns:") {
		t.Fatalf("expected TLS turn url in %v", turn.URLs)
	}
	// Deterministic: same inputs -> same credential.
	again := ICEConfigFor("user-42", TurnSettings{TurnURL: "turn:turn.x:3478", Secret: "s3cr3t", TTL: 3600}, now)
	if again.ICEServers[len(again.ICEServers)-1].Credential != turn.Credential {
		t.Fatal("HMAC credential not deterministic")
	}
}

func TestHubRelayAndPresence(t *testing.T) {
	h := NewHub()
	a := h.Join("room1", "userA", "Alice")
	// First peer's welcome lists no existing peers.
	drainType(t, a, "welcome")

	b := h.Join("room1", "userB", "Bob")
	drainType(t, b, "welcome")
	// A is notified that B joined.
	drainType(t, a, "peer-joined")

	if h.PeerCount("room1") != 2 {
		t.Fatalf("want 2 peers, got %d", h.PeerCount("room1"))
	}

	// A broadcasts an offer (no "to") -> B receives it with "from" = A.
	h.Route("room1", a, []byte(`{"type":"offer","sdp":"x"}`))
	msg := drainType(t, b, "offer")
	if !strings.Contains(msg, `"from":"`+a.ID+`"`) {
		t.Fatalf("offer missing from stamp: %s", msg)
	}

	// Targeted message: B -> A only.
	h.Route("room1", b, []byte(`{"type":"answer","to":"`+a.ID+`"}`))
	drainType(t, a, "answer")

	h.Leave("room1", b)
	drainType(t, a, "peer-left")
	if h.PeerCount("room1") != 1 {
		t.Fatalf("want 1 peer after leave, got %d", h.PeerCount("room1"))
	}
}

// drainType reads from a peer until it sees a message of the given type.
func drainType(t *testing.T, p *Peer, typ string) string {
	t.Helper()
	for {
		select {
		case raw := <-p.Out():
			if strings.Contains(string(raw), `"type":"`+typ+`"`) {
				return string(raw)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q", typ)
			return ""
		}
	}
}
