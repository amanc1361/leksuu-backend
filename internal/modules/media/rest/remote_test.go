package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leksuu/platform/internal/modules/media/store"
	"github.com/leksuu/platform/internal/modules/media/usecase"
)

func postRemote(t *testing.T, h *Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/remote", strings.NewReader(`{"url":"`+url+`"}`))
	rec := httptest.NewRecorder()
	h.ImportRemote(rec, req)
	return rec
}

// A URL copied by an earlier import is reused from storage without any network
// access: the ".invalid" host can never resolve, so a DNS lookup before the
// storage check would fail the request (a re-import once lost 12 stored files
// to a DNS hiccup this way).
func TestImportRemoteReusesStoredObjectWithoutResolvingHost(t *testing.T) {
	st, err := store.NewLocal(t.TempDir(), "https://leksuu.test")
	if err != nil {
		t.Fatal(err)
	}
	svc := usecase.NewService(st, 1<<20)
	const url = "https://cdn.invalid/media/audio/hello.mp3"
	first, err := svc.ImportRemote(context.Background(), url, "hello.mp3", func(context.Context) ([]byte, string, error) {
		return []byte("ID3 mp3 bytes"), "audio/mpeg", nil
	})
	if err != nil || first.Reused {
		t.Fatalf("seed: reused=%v err=%v", first != nil && first.Reused, err)
	}

	rec := postRemote(t, NewHandler(svc, 1<<20), url)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var got struct {
		URL    string `json:"url"`
		Reused bool   `json:"reused"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !got.Reused || got.URL != first.URL {
		t.Fatalf("got %+v (err %v), want reused %s", got, err, first.URL)
	}
}

// A host that cannot be resolved is a transient failure (5xx, retried by the
// importer), not a bad request; private addresses stay refused.
func TestImportRemoteFetchErrorsAreClassified(t *testing.T) {
	st, err := store.NewLocal(t.TempDir(), "https://leksuu.test")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(usecase.NewService(st, 1<<20), 1<<20)
	if rec := postRemote(t, h, "https://not-stored.invalid/a.mp3"); rec.Code < 500 {
		t.Fatalf("unresolvable host: status = %d, want 5xx", rec.Code)
	}
	for _, u := range []string{"http://localhost/a.mp3", "http://127.0.0.1/a.mp3", "ftp://cdn.example/a.mp3"} {
		if rec := postRemote(t, h, u); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", u, rec.Code)
		}
	}
}
