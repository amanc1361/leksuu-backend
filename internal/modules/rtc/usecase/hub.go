// Package usecase holds the live-class signaling logic: an in-memory hub that
// relays opaque JSON signaling envelopes (SDP offer/answer, ICE candidates,
// chat) between peers that share a room, plus ICE/TURN credential issuance.
//
// The hub is media-agnostic: media flows peer-to-peer (or via TURN relay); the
// server only brokers signaling. It never parses SDP — only the routing field
// "to" and stamps "from".
package usecase

import (
	"encoding/json"
	"sync"

	"github.com/leksuu/platform/internal/shared/id"
)

// Peer is one connected participant in a signaling room.
type Peer struct {
	ID     string
	UserID string
	Name   string
	send   chan []byte
	mu     sync.Mutex
	closed bool
}

// Out is the peer's outbound queue, drained by the delivery layer's write pump.
func (p *Peer) Out() <-chan []byte { return p.send }

func (p *Peer) enqueue(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	select {
	case p.send <- b:
	default: // drop on a slow/stuck consumer rather than block the hub
	}
}

func (p *Peer) closeChan() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.send)
	}
}

// Hub relays signaling between peers sharing a room.
type Hub struct {
	mu    sync.Mutex
	rooms map[string]map[string]*Peer
}

func NewHub() *Hub { return &Hub{rooms: map[string]map[string]*Peer{}} }

// Join registers a peer in a room. The newcomer receives a "welcome" listing the
// peers already present (with their identity); those peers receive "peer-joined".
func (h *Hub) Join(roomID, userID, name string) *Peer {
	p := &Peer{ID: id.New(), UserID: userID, Name: name, send: make(chan []byte, 32)}
	h.mu.Lock()
	room := h.rooms[roomID]
	if room == nil {
		room = map[string]*Peer{}
		h.rooms[roomID] = room
	}
	existing := make([]map[string]any, 0, len(room))
	for pid, peer := range room {
		existing = append(existing, map[string]any{"id": pid, "userId": peer.UserID, "name": peer.Name})
	}
	room[p.ID] = p
	h.mu.Unlock()

	p.enqueue(mustJSON(map[string]any{"type": "welcome", "selfId": p.ID, "peers": existing}))
	h.broadcast(roomID, p.ID, map[string]any{"type": "peer-joined", "from": p.ID, "userId": userID, "name": name})
	return p
}

// Leave removes a peer from its room and notifies the others.
func (h *Hub) Leave(roomID string, p *Peer) {
	h.mu.Lock()
	if room := h.rooms[roomID]; room != nil {
		delete(room, p.ID)
		if len(room) == 0 {
			delete(h.rooms, roomID)
		}
	}
	h.mu.Unlock()
	p.closeChan()
	h.broadcast(roomID, p.ID, map[string]any{"type": "peer-left", "from": p.ID})
}

// Route relays a raw client envelope. With a "to" field it is unicast to that
// peer; otherwise it is sent to every other peer in the room. "from" is stamped.
func (h *Hub) Route(roomID string, from *Peer, raw []byte) {
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	env["from"] = from.ID
	env["userId"] = from.UserID
	env["name"] = from.Name
	to, _ := env["to"].(string)
	out := mustJSON(env)

	h.mu.Lock()
	room := h.rooms[roomID]
	targets := make([]*Peer, 0, len(room))
	if to != "" {
		if peer := room[to]; peer != nil {
			targets = append(targets, peer)
		}
	} else {
		for pid, peer := range room {
			if pid != from.ID {
				targets = append(targets, peer)
			}
		}
	}
	h.mu.Unlock()
	for _, peer := range targets {
		peer.enqueue(out)
	}
}

func (h *Hub) broadcast(roomID, exceptID string, msg map[string]any) {
	out := mustJSON(msg)
	h.mu.Lock()
	room := h.rooms[roomID]
	targets := make([]*Peer, 0, len(room))
	for pid, peer := range room {
		if pid != exceptID {
			targets = append(targets, peer)
		}
	}
	h.mu.Unlock()
	for _, peer := range targets {
		peer.enqueue(out)
	}
}

// PeerCount reports how many peers are in a room (tests/metrics).
func (h *Hub) PeerCount(roomID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms[roomID])
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
