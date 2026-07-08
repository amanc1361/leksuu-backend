// Package rtc is the live-class real-time module: WebRTC signaling (a
// dependency-free WebSocket hub) and self-hosted ICE/TURN credential issuance.
// Media flows peer-to-peer; a coturn instance provides STUN + TURN relay for
// NAT traversal. No external/paid service is required.
package rtc

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/rtc/rest"
	"github.com/leksuu/platform/internal/modules/rtc/usecase"
)

type Deps struct {
	Guard       rest.Guard       // header-based auth for /rtc/ice
	TokenParser rest.TokenParser // validates the WS ?token=
	Authorizer  rest.Authorizer  // is this user a participant of the booking?
	Identifier  rest.Identifier  // resolves display names for the roster/chat
	StunURL     string
	TurnURL     string
	TurnTLSURL  string
	TurnSecret  string
	TurnTTL     int
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
}

func New(d Deps) *Module {
	hub := usecase.NewHub()
	turn := usecase.TurnSettings{
		StunURL: d.StunURL, TurnURL: d.TurnURL, TurnTLSURL: d.TurnTLSURL,
		Secret: d.TurnSecret, TTL: d.TurnTTL,
	}
	return &Module{
		handler: rest.NewHandler(hub, turn, d.TokenParser, d.Authorizer, d.Identifier),
		guard:   d.Guard,
	}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }
