//go:build s3

package store

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/media/domain"
)

// fakeS3 is just enough of the S3 HTTP API for the adapter to drive: it records
// the object path it was asked for, so the tests can assert how a key is mapped
// into the bucket. Signatures are not checked - minio-go's signing is not what
// these tests are about.
type fakeS3 struct {
	body    []byte
	ct      string
	missing bool

	gotPath   string
	gotMethod string
	putBody   []byte
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.gotPath, f.gotMethod = r.URL.Path, r.Method

	if r.Method == http.MethodPut {
		f.putBody, _ = io.ReadAll(r.Body)
		w.Header().Set("ETag", `"d41d8cd98f00b204e9800998ecf8427e"`)
		w.WriteHeader(http.StatusOK)
		return
	}
	if f.missing {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error>`+
			`<Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
		return
	}
	w.Header().Set("Content-Type", f.ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(f.body)))
	w.Header().Set("Last-Modified", time.Unix(1700000000, 0).UTC().Format(http.TimeFormat))
	w.Header().Set("ETag", `"abc"`)
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(f.body)
	}
}

func newTestS3(t *testing.T, f *fakeS3, prefix string, publicRead bool, publicBase string) *S3 {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)

	s, err := NewS3(strings.TrimPrefix(srv.URL, "http://"), "us-east-1", "bucket", prefix,
		"key", "secret", publicBase, false, publicRead)
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	return s
}

// A private bucket must hand back an API URL, and the prefix must stay out of
// it - the prefix is storage layout, re-applied on the way back in by Get.
func TestPutPrivateBucketReturnsAPIURL(t *testing.T) {
	f := &fakeS3{}
	s := newTestS3(t, f, "leksuu", false, "https://leksuu.ir")

	got, err := s.Put(context.Background(), "a/b.png", "image/png", []byte("xyz"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if want := "https://leksuu.ir/media/a/b.png"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
	if want := "/bucket/leksuu/a/b.png"; f.gotPath != want {
		t.Fatalf("object path = %q, want %q", f.gotPath, want)
	}
	// Over plain HTTP minio-go uploads with aws-chunked streaming signatures, so
	// the recorded body carries chunk framing around the payload.
	if !strings.Contains(string(f.putBody), "xyz") {
		t.Fatalf("payload missing from stored body %q", f.putBody)
	}
}

// A bucket opened for anonymous reads keeps the old behaviour: URLs point
// straight at the bucket, prefix included, and never touch the API.
func TestPutPublicBucketReturnsBucketURL(t *testing.T) {
	f := &fakeS3{}
	s := newTestS3(t, f, "leksuu", true, "https://cdn.example.com")

	got, err := s.Put(context.Background(), "a/b.png", "image/png", []byte("xyz"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if want := "https://cdn.example.com/leksuu/a/b.png"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}

func TestPutNoPrefix(t *testing.T) {
	f := &fakeS3{}
	s := newTestS3(t, f, "", false, "https://leksuu.ir")

	got, _ := s.Put(context.Background(), "b.png", "image/png", []byte("x"))
	if want := "https://leksuu.ir/media/b.png"; got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
	if want := "/bucket/b.png"; f.gotPath != want {
		t.Fatalf("object path = %q, want %q", f.gotPath, want)
	}
}

func TestGetReturnsBlob(t *testing.T) {
	f := &fakeS3{body: []byte("hello"), ct: "image/png"}
	s := newTestS3(t, f, "leksuu", false, "https://leksuu.ir")

	blob, err := s.Get(context.Background(), "a/b.png")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer blob.Body.Close()

	if want := "/bucket/leksuu/a/b.png"; f.gotPath != want {
		t.Fatalf("fetched path = %q, want %q", f.gotPath, want)
	}
	if blob.ContentType != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", blob.ContentType)
	}
	if blob.Size != 5 {
		t.Fatalf("Size = %d, want 5", blob.Size)
	}
	got, err := io.ReadAll(blob.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("body = %q, want hello", got)
	}
}

// The handler turns this into a 404 rather than a 502, so the distinction has
// to survive the adapter.
func TestGetMissingKeyIsErrNotFound(t *testing.T) {
	f := &fakeS3{missing: true}
	s := newTestS3(t, f, "", false, "https://leksuu.ir")

	if _, err := s.Get(context.Background(), "nope.png"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

// A key may not climb out of the configured prefix.
func TestKeyTraversalStaysInsidePrefix(t *testing.T) {
	f := &fakeS3{body: []byte("x"), ct: "text/plain"}
	s := newTestS3(t, f, "leksuu", false, "https://leksuu.ir")

	blob, err := s.Get(context.Background(), "../../etc/passwd")
	if err == nil {
		blob.Body.Close()
	}
	if strings.Contains(f.gotPath, "..") {
		t.Fatalf("traversal reached the bucket: %q", f.gotPath)
	}
	if !strings.HasPrefix(f.gotPath, "/bucket/leksuu/") {
		t.Fatalf("path %q escaped the prefix", f.gotPath)
	}
}

var _ domain.Reader = (*S3)(nil)
var _ domain.Store = (*S3)(nil)

// Remote imports skip the download when Stat finds the deterministic key.
func TestStatReportsExistingAndMissingObjects(t *testing.T) {
	f := &fakeS3{body: []byte("mp3"), ct: "audio/mpeg"}
	s := newTestS3(t, f, "leksuu", false, "https://api.example")
	ct, err := s.Stat(context.Background(), "remote/abc.mp3")
	if err != nil || ct != "audio/mpeg" || f.gotMethod != http.MethodHead || f.gotPath != "/bucket/leksuu/remote/abc.mp3" {
		t.Fatalf("stat existing: ct=%q err=%v %s %s", ct, err, f.gotMethod, f.gotPath)
	}
	if got := s.URL("remote/abc.mp3"); got != "https://api.example/media/remote/abc.mp3" {
		t.Fatalf("url = %q", got)
	}
	missing := newTestS3(t, &fakeS3{missing: true}, "leksuu", false, "https://api.example")
	if _, err := missing.Stat(context.Background(), "remote/none.mp3"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing object: err=%v, want ErrNotFound", err)
	}
}
