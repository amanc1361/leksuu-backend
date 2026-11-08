package rest

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/media/domain"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

// PlaybackAsset is resolved from an authorized lesson item, never from a
// client-supplied object key. HLS grants cover only the item's HLS directory.
type PlaybackAsset struct {
	Key string
	HLS bool
}

type playbackTicket struct {
	Key     string `json:"k"`
	Subject string `json:"s"`
	Expires int64  `json:"e"`
	HLS     bool   `json:"h"`
}

func (h *Handler) ConfigurePlayback(secret []byte, base string, cdn bool,
	access func(context.Context, string, string, bool) (PlaybackAsset, error),
	owned KeyOwnership, store domain.Store) {
	h.playbackKey = append([]byte(nil), secret...)
	h.playbackBase = strings.TrimRight(base, "/")
	h.playbackCDN = cdn
	h.playbackAccess = access
	h.keyOwnership = owned
	if rd, ok := store.(domain.Reader); ok {
		h.playlistReader = rd
	}
	if dir, ok := store.(domain.LocalDir); ok {
		h.playlistDir = dir.Dir()
	}
}

func (h *Handler) signPlayback(t playbackTicket) string {
	data, _ := json.Marshal(t)
	mac := hmac.New(sha256.New, h.playbackKey)
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (h *Handler) validPlaybackTicket(raw, key string) bool {
	if len(h.playbackKey) < 32 || len(raw) > 4096 {
		return false
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return false
	}
	data, e1 := base64.RawURLEncoding.DecodeString(parts[0])
	sig, e2 := base64.RawURLEncoding.DecodeString(parts[1])
	if e1 != nil || e2 != nil {
		return false
	}
	mac := hmac.New(sha256.New, h.playbackKey)
	mac.Write(data)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return false
	}
	var t playbackTicket
	if json.Unmarshal(data, &t) != nil || t.Subject == "" || t.Expires <= time.Now().Unix() {
		return false
	}
	if t.HLS {
		return strings.HasPrefix(key, strings.TrimSuffix(t.Key, "/")+"/")
	}
	return key == t.Key
}

// Called only after signature validation. A shared segment cache must expire
// before the bearer grant does, including requests made at the end of its TTL.
func playbackCacheSeconds(raw string) int {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return 0
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0
	}
	var ticket playbackTicket
	if json.Unmarshal(data, &ticket) != nil {
		return 0
	}
	seconds := int(ticket.Expires - time.Now().Unix() - 1)
	if seconds < 0 {
		return 0
	}
	if seconds > 30 {
		return 30
	}
	return seconds
}

// PlaybackGrant checks the current learner and target language through the
// injected course/billing service before minting a short-lived bearer URL.
func (h *Handler) PlaybackGrant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	p, ok := authn.FromContext(r.Context())
	if !ok || h.playbackAccess == nil || len(h.playbackKey) < 32 {
		http.Error(w, "playback unavailable", http.StatusServiceUnavailable)
		return
	}
	asset, err := h.playbackAccess(r.Context(), p.Subject, r.PathValue("item"), p.HasRole("admin"))
	if err != nil || asset.Key == "" {
		http.Error(w, "playback unavailable", http.StatusForbidden)
		return
	}
	key := strings.TrimPrefix(path.Clean("/"+asset.Key), "/")
	if key != asset.Key || strings.Contains(asset.Key, "..") {
		http.Error(w, "invalid asset", http.StatusBadRequest)
		return
	}
	exp := time.Now().Add(10 * time.Minute).Unix()
	ticket := h.signPlayback(playbackTicket{Key: key, Subject: p.Subject, Expires: exp, HLS: asset.HLS})
	entry := key
	if asset.HLS {
		entry += "/master.m3u8"
	}
	web.JSON(w, http.StatusOK, map[string]any{
		"url":        h.playbackBase + "/media/" + entry + "?playback=" + url.QueryEscape(ticket),
		"expires_at": exp, "hls": asset.HLS,
	})
}

// AdminPreviewGrant permits review of a source while its Media Library item is
// being assembled. Only keys created in the protected upload namespaces qualify.
func (h *Handler) AdminPreviewGrant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	p, ok := authn.FromContext(r.Context())
	key := r.URL.Query().Get("key")
	if !ok || !p.HasRole("admin") || len(h.playbackKey) < 32 ||
		!(strings.HasPrefix(key, "protected/") || strings.HasPrefix(key, "large/")) ||
		strings.TrimPrefix(path.Clean("/"+key), "/") != key {
		http.Error(w, "preview unavailable", http.StatusForbidden)
		return
	}
	exp := time.Now().Add(10 * time.Minute).Unix()
	ticket := h.signPlayback(playbackTicket{Key: key, Subject: p.Subject, Expires: exp})
	web.JSON(w, http.StatusOK, map[string]any{"url": h.playbackBase + "/media/" + key + "?playback=" + url.QueryEscape(ticket), "expires_at": exp})
}

// Playlist children inherit the same scoped grant. Unknown, absolute and
// traversal references fail closed; the worker emits only relative paths.
func (h *Handler) servePlaylist(w http.ResponseWriter, r *http.Request, key string) {
	var body io.ReadCloser
	if h.playlistReader != nil {
		blob, err := h.playlistReader.Get(r.Context(), key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		body = blob.Body
	} else if h.playlistDir != "" {
		file, err := os.Open(path.Join(h.playlistDir, key))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		body = file
	} else {
		http.NotFound(w, r)
		return
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		http.Error(w, "playlist unavailable", http.StatusBadGateway)
		return
	}
	var out strings.Builder
	scan := bufio.NewScanner(strings.NewReader(string(data)))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "#") && strings.Contains(strings.ToUpper(line), "URI=") {
			http.Error(w, "unsupported playlist reference", http.StatusBadGateway)
			return
		}
		if line != "" && !strings.HasPrefix(line, "#") {
			if strings.ContainsAny(line, ":\\?#") || strings.HasPrefix(line, "/") || line == "." || line == ".." || strings.Contains(line, "../") {
				http.Error(w, "unsafe playlist", http.StatusBadGateway)
				return
			}
			child := path.Clean(path.Join(path.Dir(key), line))
			if !h.validPlaybackTicket(r.URL.Query().Get("playback"), child) {
				http.Error(w, "unsafe playlist", http.StatusBadGateway)
				return
			}
			line = h.playbackBase + "/media/" + child + "?playback=" + url.QueryEscape(r.URL.Query().Get("playback"))
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if scan.Err() != nil {
		http.Error(w, "playlist unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.WriteString(w, out.String())
}
