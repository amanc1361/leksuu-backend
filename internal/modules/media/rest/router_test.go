package rest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/media/domain"
)

type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

// fakeReader stands in for a private bucket: it hands back a fixed object, or
// whatever error the test wants Get to fail with.
type fakeReader struct {
	body []byte
	ct   string
	err  error
	gotK string
}

func (f *fakeReader) Get(_ context.Context, key string) (*domain.Blob, error) {
	f.gotK = key
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Blob{
		Body:        nopSeekCloser{bytes.NewReader(f.body)},
		ContentType: f.ct,
		Size:        int64(len(f.body)),
		ModTime:     time.Unix(1700000000, 0).UTC(),
	}, nil
}

// serve routes through a real ServeMux so the {file...} wildcard is populated
// exactly as it is in production.
func serve(t *testing.T, rd domain.Reader, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /media/{file...}", serveBlob(rd))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestServeBlobFullObject(t *testing.T) {
	rd := &fakeReader{body: []byte("hello-media"), ct: "image/png"}
	rec := serve(t, rd, httptest.NewRequest("GET", "/media/abc.png", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "hello-media" {
		t.Fatalf("body = %q, want %q", got, "hello-media")
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if rd.gotK != "abc.png" {
		t.Fatalf("store received key %q, want abc.png", rd.gotK)
	}
}

// Seeking inside a recording depends on this: without Range support a <video>
// can only play from the start.
func TestServeBlobRangeRequest(t *testing.T) {
	rd := &fakeReader{body: []byte("0123456789"), ct: "video/webm"}
	req := httptest.NewRequest("GET", "/media/rec-1.webm", nil)
	req.Header.Set("Range", "bytes=2-5")
	rec := serve(t, rd, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", rec.Code)
	}
	if got := rec.Body.String(); got != "2345" {
		t.Fatalf("body = %q, want %q", got, "2345")
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q, want bytes 2-5/10", got)
	}
}

// A nested key must reach the store whole — the prefix layout lives below it.
func TestServeBlobNestedKey(t *testing.T) {
	rd := &fakeReader{body: []byte("x"), ct: "audio/mpeg"}
	serve(t, rd, httptest.NewRequest("GET", "/media/course/unit1/a.mp3", nil))
	if rd.gotK != "course/unit1/a.mp3" {
		t.Fatalf("store received key %q, want course/unit1/a.mp3", rd.gotK)
	}
}

func TestServeBlobErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"missing object is 404", domain.ErrNotFound, http.StatusNotFound},
		{"backend failure is 502", errors.New("credentials rejected"), http.StatusBadGateway},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rd := &fakeReader{err: c.err}
			rec := serve(t, rd, httptest.NewRequest("GET", "/media/gone.png", nil))
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
			// The bucket's own error text must never reach the caller.
			if bytes.Contains(rec.Body.Bytes(), []byte("credentials")) {
				t.Fatalf("backend error leaked to client: %q", rec.Body.String())
			}
		})
	}
}

// Mount must expose the byte route for a private bucket, not just for local disk.
func TestMountRegistersReaderRoute(t *testing.T) {
	mux := http.NewServeMux()
	h := NewHandler(nil, 0)
	h.Mount(mux, "/api/v1", stubGuard{}, &fakeStore{fakeReader{body: []byte("ok"), ct: "text/plain"}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/media/f.txt", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 - GET /media route not mounted for a Reader store", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got == "" {
		t.Fatal("Cache-Control missing: the route skipped the longCache wrapper")
	}
}

// fakeStore satisfies domain.Store and domain.Reader, like the S3 adapter does.
type fakeStore struct{ fakeReader }

func (*fakeStore) Put(context.Context, string, string, []byte) (string, error) { return "", nil }

type stubGuard struct{}

func (stubGuard) Require(next http.Handler) http.Handler               { return next }
func (stubGuard) RequireRole(_ string, next http.Handler) http.Handler { return next }

var _ io.ReadSeekCloser = nopSeekCloser{}
