//go:build s3

package store

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"
)

func TestAdditionalMultipartContainerFormats(t *testing.T) {
	for mime, extension := range map[string]string{"audio/flac": ".flac", "audio/x-flac": ".flac", "audio/aac": ".aac", "audio/opus": ".opus", "video/x-matroska": ".mkv", "video/x-msvideo": ".avi", "video/mpeg": ".mpeg", "video/ogg": ".ogv", "audio/wave": ".wav"} {
		if got := multipartExt(mime); got != extension {
			t.Errorf("%s stored with %q; want %q", mime, got, extension)
		}
	}
	if multipartExt("text/html") != "" {
		t.Fatal("non-media multipart input must remain rejected")
	}
}

func TestMultipartTicketBindsOwnerAndExpiry(t *testing.T) {
	s := &S3{ticketKey: []byte("test-secret")}
	ticket := multipartTicket{Key: "large/fixture.mp4", UploadID: "upload-1", Subject: "admin-1",
		ContentType: "video/mp4", Size: multipartPartSize + 1, Expires: time.Now().Add(time.Hour).Unix()}
	raw, err := s.signMultipartTicket(ticket)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := s.parseMultipartTicket(raw, "admin-1")
	if err != nil || decoded.UploadID != ticket.UploadID {
		t.Fatalf("valid ticket: %v", err)
	}
	if _, err := s.parseMultipartTicket(raw, "admin-2"); !errors.Is(err, ErrMultipartTicket) {
		t.Fatal("ticket crossed administrator boundary")
	}
	if _, err := s.parseMultipartTicket(raw+"x", "admin-1"); !errors.Is(err, ErrMultipartTicket) {
		t.Fatal("tampered ticket accepted")
	}
	ticket.Expires = time.Now().Add(-time.Minute).Unix()
	raw, _ = s.signMultipartTicket(ticket)
	if _, err := s.parseMultipartTicket(raw, "admin-1"); !errors.Is(err, ErrMultipartTicket) {
		t.Fatal("expired ticket accepted")
	}
}

func TestMultipartMimeWhitelist(t *testing.T) {
	for _, ct := range []string{"video/mp4", "video/webm", "audio/mpeg", "audio/ogg"} {
		if multipartExt(ct) == "" {
			t.Fatalf("rejected %s", ct)
		}
	}
	for _, ct := range []string{"image/svg+xml", "text/html", "application/javascript", ""} {
		if multipartExt(ct) != "" {
			t.Fatalf("accepted %s", ct)
		}
	}
}

func TestMultipartPresignKeepsPartAndUploadID(t *testing.T) {
	s, err := NewS3("storage.example", "us-east-1", "media", "prefix", "access", "secret", "https://api.example", true, false)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := s.signMultipartTicket(multipartTicket{Key: "large/fixture.mp4", UploadID: "upload-123", Subject: "admin-1",
		ContentType: "video/mp4", Size: multipartPartSize + 1, Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.MultipartPartURL(context.Background(), ticket, "admin-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Query().Get("uploadId") != "upload-123" || u.Query().Get("partNumber") != "2" || u.Query().Get("X-Amz-Signature") == "" {
		t.Fatalf("bad presigned URL: %s, %v", raw, err)
	}
	if _, err := s.MultipartPartURL(context.Background(), ticket, "admin-1", 3); !errors.Is(err, ErrMultipartTicket) {
		t.Fatal("out-of-range part signed")
	}
}
