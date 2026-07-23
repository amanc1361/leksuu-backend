// Package rest is the live-class signaling delivery layer: an ICE-config
// endpoint (header-authenticated) and a WebSocket signaling endpoint
// (query-token authenticated, since browsers can't set WS headers).
package rest

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/rtc/usecase"
	"github.com/leksuu/platform/internal/modules/rtc/ws"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Guard interface {
	Require(next http.Handler) http.Handler
}

// TokenParser validates a raw access token (from the WS query string).
type TokenParser interface {
	ParseToken(raw string) (authn.Principal, error)
}

// Authorizer decides if a user may join a booking's signaling room, returning
// the room id to use.
type Authorizer interface {
	CanJoin(ctx context.Context, userID, bookingID string) (bool, string, error)
}

// Identifier resolves a user's display name for the room roster/chat (best-effort).
type Identifier interface {
	DisplayName(ctx context.Context, userID string) string
}

type Handler struct {
	hub    *usecase.Hub
	turn   usecase.TurnSettings
	parser TokenParser
	authz  Authorizer
	ident  Identifier
	clock  func() time.Time
}

func NewHandler(hub *usecase.Hub, turn usecase.TurnSettings, parser TokenParser, authz Authorizer, ident Identifier) *Handler {
	return &Handler{hub: hub, turn: turn, parser: parser, authz: authz, ident: ident, clock: time.Now}
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("GET "+prefix+"/rtc/ice", guard.Require(http.HandlerFunc(h.ICE)))
	mux.Handle("GET "+prefix+"/rtc/signal", http.HandlerFunc(h.Signal))
}

// ICE returns the per-user ICE server configuration (STUN + time-limited TURN).
func (h *Handler) ICE(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	web.JSON(w, http.StatusOK, usecase.ICEConfigFor(p.Subject, h.turn, h.clock()))
}

// Signal upgrades to WebSocket and joins the caller to the booking's room.
func (h *Handler) Signal(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	bookingID := r.URL.Query().Get("booking")
	if token == "" || bookingID == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	principal, err := h.parser.ParseToken(token)
	if err != nil {
		web.Error(w, r, apperr.New("rtc.unauthorized", apperr.KindUnauthorized))
		return
	}
	ok, roomID, err := h.authz.CanJoin(r.Context(), principal.Subject, bookingID)
	if err != nil || !ok {
		web.Error(w, r, apperr.New("rtc.forbidden", apperr.KindForbidden))
		return
	}

	conn, err := ws.Upgrade(w, r)
	if err != nil {
		// A silent return leaves the browser with an opaque handshake failure
		// and nothing in the logs. Report it instead: this is what a reverse
		// proxy that strips Upgrade headers (or serves the backend over HTTP/2,
		// where hijacking is unavailable) looks like in production.
		slog.WarnContext(r.Context(), "rtc: websocket upgrade failed",
			"err", err, "proto", r.Proto,
			"upgrade", r.Header.Get("Upgrade"), "connection", r.Header.Get("Connection"))
		web.Error(w, r, apperr.New("rtc.upgrade_failed", apperr.KindValidation))
		return
	}
	defer conn.Close()

	name := ""
	if h.ident != nil {
		name = h.ident.DisplayName(r.Context(), principal.Subject)
	}
	peer := h.hub.Join(roomID, principal.Subject, name)
	defer h.hub.Leave(roomID, peer)

	// quit lets the read pump stop the write pump immediately on disconnect, so
	// the peer-left broadcast fires at once (not on the next 25s keepalive tick).
	quit := make(chan struct{})
	go func() {
		ping := time.NewTicker(25 * time.Second)
		defer ping.Stop()
		for {
			select {
			case msg, ok := <-peer.Out():
				if !ok || conn.WriteText(msg) != nil {
					conn.Close()
					return
				}
			case <-ping.C:
				if conn.WritePing() != nil {
					conn.Close()
					return
				}
			case <-quit:
				return
			}
		}
	}()

	// Read pump: relay inbound messages until the socket closes.
	for {
		msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		h.hub.Route(roomID, peer, msg)
	}
	close(quit)
	conn.Close()
}
