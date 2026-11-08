package rest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/media/domain"
)

type playbackReader struct{ files map[string][]byte }

func (r playbackReader) Put(context.Context, string, string, []byte) (string, error) { return "", nil }

func (r playbackReader) Get(_ context.Context, key string) (*domain.Blob, error) {
	data, ok := r.files[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	ct := "video/mp2t"
	if strings.HasSuffix(key, ".m3u8") {
		ct = "application/vnd.apple.mpegurl"
	}
	return &domain.Blob{Body: nopSeekCloser{bytes.NewReader(data)}, Size: int64(len(data)), ContentType: ct, ModTime: time.Now()}, nil
}

func TestProtectedBytesRejectAnonymousAndTamperedGrant(t *testing.T) {
	h := NewHandler(nil, 0)
	h.ConfigurePlayback([]byte(strings.Repeat("s", 32)), "https://cdn.example", true, nil,
		func(context.Context, string) (bool, error) { return true, nil }, playbackReader{files: map[string][]byte{"large/movie.mp4": []byte("0123456789")}})
	mux := http.NewServeMux()
	mux.Handle("GET /media/{file...}", longCache(h.authorizeBytes(serveBlob(h.playlistReader))))
	request := func(raw string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/media/large/movie.mp4"+raw, nil)
		req.Header.Set("Range", "bytes=2-5")
		mux.ServeHTTP(rec, req)
		return rec
	}
	if got := request(""); got.Code != 403 || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("anonymous status=%d cache=%q", got.Code, got.Header().Get("Cache-Control"))
	}
	ticket := h.signPlayback(playbackTicket{Key: "large/movie.mp4", Subject: "user-1", Expires: time.Now().Add(time.Minute).Unix()})
	if got := request("?playback=" + ticket); got.Code != 206 || got.Body.String() != "2345" || !strings.Contains(got.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("authorized range status=%d body=%q cache=%q", got.Code, got.Body.String(), got.Header().Get("Cache-Control"))
	}
	if got := request("?playback=" + ticket + "x"); got.Code != 403 {
		t.Fatalf("tampered status=%d", got.Code)
	}
	expired := h.signPlayback(playbackTicket{Key: "large/movie.mp4", Subject: "user-1", Expires: time.Now().Add(-time.Second).Unix()})
	if got := request("?playback=" + expired); got.Code != 403 {
		t.Fatalf("expired status=%d", got.Code)
	}
}

func TestLargeStoryPodcastRemainReadableButLicensedLargeIsProtected(t *testing.T) {
	files := map[string][]byte{
		"large/story.mp3":              []byte("story"),
		"large/legacy-film.mp4":        []byte("film"),
		"protected/large/new-film.mp4": []byte("film"),
	}
	h := NewHandler(nil, 0)
	h.ConfigurePlayback([]byte(strings.Repeat("s", 32)), "https://api.example", false, nil,
		func(_ context.Context, key string) (bool, error) { return key == "large/legacy-film.mp4", nil }, playbackReader{files: files})
	mux := http.NewServeMux()
	mux.Handle("GET /media/{file...}", longCache(h.authorizeBytes(serveBlob(h.playlistReader))))
	get := func(key string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/media/"+key, nil))
		return rec.Code
	}
	if got := get("large/story.mp3"); got != 200 {
		t.Fatalf("legacy Story/Podcast upload became inaccessible: %d", got)
	}
	if got := get("large/legacy-film.mp4"); got != 403 {
		t.Fatalf("legacy Film upload exposed: %d", got)
	}
	if got := get("protected/large/new-film.mp4"); got != 403 {
		t.Fatalf("new Film upload exposed: %d", got)
	}
}

func TestHLSPlaylistChildrenStayInsideGrant(t *testing.T) {
	files := map[string][]byte{
		"hls/item/v1/master.m3u8":       []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=500000\nv360/index.m3u8\n"),
		"hls/item/v1/v360/index.m3u8":   []byte("#EXTM3U\n#EXTINF:4.0,\nseg_00000.ts\n"),
		"hls/item/v1/v360/seg_00000.ts": []byte("transport"),
	}
	h := NewHandler(nil, 0)
	h.ConfigurePlayback([]byte(strings.Repeat("k", 32)), "https://cdn.example", true, nil, nil, playbackReader{files: files})
	mux := http.NewServeMux()
	mux.Handle("GET /media/{file...}", longCache(h.authorizeBytes(serveBlob(h.playlistReader))))
	ticket := h.signPlayback(playbackTicket{Key: "hls/item/v1", Subject: "learner", Expires: time.Now().Add(time.Minute).Unix(), HLS: true})
	get := func(key string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/media/"+key+"?playback="+ticket, nil))
		return rec
	}
	if got := get("hls/item/v1/master.m3u8"); got.Code != 200 || !strings.Contains(got.Body.String(), "https://cdn.example/media/hls/item/v1/v360/index.m3u8?playback=") {
		t.Fatalf("master status=%d body=%q", got.Code, got.Body.String())
	}
	if got := get("hls/item/v1/v360/seg_00000.ts"); got.Code != 200 || got.Body.String() != "transport" || !strings.Contains(got.Header().Get("Cache-Control"), "s-maxage=30") {
		t.Fatalf("segment status=%d cache=%q", got.Code, got.Header().Get("Cache-Control"))
	}
	if got := get("hls/other/v1/seg_00000.ts"); got.Code != 403 {
		t.Fatalf("other item status=%d", got.Code)
	}
	short := h.signPlayback(playbackTicket{Key: "hls/item/v1", Subject: "learner", Expires: time.Now().Add(2 * time.Second).Unix(), HLS: true})
	shortRec := httptest.NewRecorder()
	mux.ServeHTTP(shortRec, httptest.NewRequest("GET", "/media/hls/item/v1/v360/seg_00000.ts?playback="+short, nil))
	if shortRec.Code != 200 || strings.Contains(shortRec.Header().Get("Cache-Control"), "s-maxage=30") {
		t.Fatalf("near-expiry grant outlived shared cache: status=%d cache=%q", shortRec.Code, shortRec.Header().Get("Cache-Control"))
	}
	files["hls/item/v1/master.m3u8"] = []byte("#EXTM3U\n../../other/secret.ts\n")
	if got := get("hls/item/v1/master.m3u8"); got.Code != 502 {
		t.Fatalf("unsafe playlist status=%d", got.Code)
	}
}
