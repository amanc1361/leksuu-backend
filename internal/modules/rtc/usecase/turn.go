package usecase

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// TurnSettings configure the self-hosted ICE servers (a coturn instance providing
// both STUN and TURN). When TurnSecret is empty, only STUN is advertised.
type TurnSettings struct {
	StunURL    string // one or more STUN URLs, comma-separated (e.g. stun:turn.leksuu.app:3478)
	TurnURL    string // e.g. turn:turn.leksuu.app:3478
	TurnTLSURL string // e.g. turns:turn.leksuu.app:5349
	Secret     string // coturn static-auth-secret (TURN REST API)
	TTL        int    // credential lifetime in seconds
}

// ICEServer matches the browser RTCIceServer shape.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type ICEConfig struct {
	ICEServers []ICEServer `json:"iceServers"`
}

// ICEConfigFor builds the per-user ICE configuration. TURN credentials are
// time-limited and derived per the coturn TURN REST API convention:
//
//	username   = "<unix-expiry>:<userID>"
//	credential = base64(HMAC-SHA1(secret, username))
//
// coturn validates these when configured with `use-auth-secret` +
// `static-auth-secret=<secret>`, so no per-user secret is ever shipped.
func ICEConfigFor(userID string, s TurnSettings, now time.Time) ICEConfig {
	cfg := ICEConfig{ICEServers: []ICEServer{}}
	// Several STUN URLs may be configured; browsers race them, so listing more
	// than one improves the odds on networks where a given host is unreachable.
	if stun := splitURLs(s.StunURL); len(stun) > 0 {
		cfg.ICEServers = append(cfg.ICEServers, ICEServer{URLs: stun})
	}
	urls := []string{}
	if s.TurnURL != "" {
		urls = append(urls, s.TurnURL)
	}
	if s.TurnTLSURL != "" {
		urls = append(urls, s.TurnTLSURL)
	}
	if s.Secret != "" && len(urls) > 0 {
		ttl := s.TTL
		if ttl <= 0 {
			ttl = 86400
		}
		expiry := now.Add(time.Duration(ttl) * time.Second).Unix()
		username := strconv.FormatInt(expiry, 10) + ":" + userID
		mac := hmac.New(sha1.New, []byte(s.Secret))
		mac.Write([]byte(username))
		credential := base64.StdEncoding.EncodeToString(mac.Sum(nil))
		cfg.ICEServers = append(cfg.ICEServers, ICEServer{URLs: urls, Username: username, Credential: credential})
	}
	return cfg
}

// splitURLs parses a comma-separated URL list, ignoring blanks.
func splitURLs(v string) []string {
	out := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
