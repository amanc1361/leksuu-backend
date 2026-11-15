package rest

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/leksuu/platform/internal/modules/media/usecase"
)

func uploadFixture(t *testing.T, filename, contentType string, data []byte, maxBytes int64) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/media", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	NewHandler(usecase.NewService(&fakeStore{}, maxBytes), maxBytes).Upload(recorder, request)
	return recorder
}

func TestDirectMediaUploadAcceptsSupportedAudioAndVideo(t *testing.T) {
	for _, fixture := range []struct{ filename, contentType string }{
		{"episode.mp3", "audio/mpeg"}, {"song.flac", "audio/flac"}, {"film.mp4", "video/mp4"},
	} {
		recorder := uploadFixture(t, fixture.filename, fixture.contentType, []byte("synthetic media bytes"), 1<<20)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("%s: status %d, body %s", fixture.filename, recorder.Code, recorder.Body.String())
		}
		var result struct {
			ContentType string `json:"content_type"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || result.ContentType != fixture.contentType {
			t.Fatalf("%s: result %+v, error %v", fixture.filename, result, err)
		}
	}
}

func TestDirectMediaUploadRejectsUnsupportedAndOversized(t *testing.T) {
	if got := uploadFixture(t, "active.html", "text/html", []byte("<html></html>"), 1<<20); got.Code < 400 {
		t.Fatalf("unsupported upload status %d", got.Code)
	}
	if got := uploadFixture(t, "episode.mp3", "audio/mpeg", []byte("larger than limit"), 3); got.Code < 400 {
		t.Fatalf("oversized upload status %d", got.Code)
	}
}
