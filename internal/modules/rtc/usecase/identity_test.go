package usecase

import (
	"strings"
	"testing"
)

// Identity must reach the roster: welcome lists existing peers with their name,
// peer-joined carries the newcomer's name, and relayed messages (chat) are
// stamped with the sender's name + userId so chat/Q&A can attribute them.
func TestHubStampsIdentity(t *testing.T) {
	h := NewHub()
	a := h.Join("room1", "userA", "Alice")
	drainType(t, a, "welcome")

	b := h.Join("room1", "userB", "Bob")
	// B's welcome should list A with name "Alice" and userId "userA".
	welcome := drainType(t, b, "welcome")
	if !strings.Contains(welcome, `"name":"Alice"`) || !strings.Contains(welcome, `"userId":"userA"`) {
		t.Fatalf("welcome missing peer identity: %s", welcome)
	}

	// A is notified B joined, with B's identity.
	joined := drainType(t, a, "peer-joined")
	if !strings.Contains(joined, `"name":"Bob"`) || !strings.Contains(joined, `"userId":"userB"`) {
		t.Fatalf("peer-joined missing identity: %s", joined)
	}

	// A chat from B reaches A stamped with B's name + userId (for attribution).
	h.Route("room1", b, []byte(`{"type":"chat","text":"hi"}`))
	chat := drainType(t, a, "chat")
	if !strings.Contains(chat, `"name":"Bob"`) || !strings.Contains(chat, `"userId":"userB"`) {
		t.Fatalf("chat missing sender identity: %s", chat)
	}
}

// The host's speak-invite must relay like mute-request: broadcast reaches every
// other peer; a targeted one reaches only the addressed peer.
func TestHubSpeakInviteRelay(t *testing.T) {
	h := NewHub()
	host := h.Join("room1", "host", "Teacher")
	drainType(t, host, "welcome")
	s1 := h.Join("room1", "s1", "Sara")
	drainType(t, s1, "welcome")
	drainType(t, host, "peer-joined")
	s2 := h.Join("room1", "s2", "Kaveh")
	drainType(t, s2, "welcome")
	drainType(t, host, "peer-joined")
	drainType(t, s1, "peer-joined")

	// Broadcast enable-all -> both students receive it.
	h.Route("room1", host, []byte(`{"type":"speak-invite","mic":true,"cam":true}`))
	if e := drainType(t, s1, "speak-invite"); !strings.Contains(e, `"mic":true`) {
		t.Fatalf("s1 speak-invite malformed: %s", e)
	}
	drainType(t, s2, "speak-invite")

	// Targeted allow -> only s2 receives it.
	h.Route("room1", host, []byte(`{"type":"speak-invite","to":"`+s2.ID+`","mic":true,"cam":true}`))
	got := drainType(t, s2, "speak-invite")
	if !strings.Contains(got, `"to":"`+s2.ID+`"`) {
		t.Fatalf("targeted speak-invite missing addressee: %s", got)
	}
	// s1 must NOT get the targeted one (only the earlier broadcast, already drained).
	select {
	case raw := <-s1.Out():
		if strings.Contains(string(raw), "speak-invite") {
			t.Fatalf("s1 wrongly received targeted speak-invite: %s", raw)
		}
	default:
	}
}
