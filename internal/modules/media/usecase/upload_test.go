package usecase

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/leksuu/platform/internal/modules/media/store"
)

const htmlFixture = "<!DOCTYPE html><html><body>inert audit fixture</body></html>"

func newLocalSvc(t *testing.T) *Service {
	st, err := store.NewLocal(t.TempDir(), "https://leksuu.test")
	if err != nil {
		t.Fatal(err)
	}
	return NewService(st, 1<<20)
}

func pngBytes(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(1, 1, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAvatarRejectsHTMLDisguisedAsImage(t *testing.T) {
	svc := newLocalSvc(t)
	if _, err := svc.UploadAvatar(context.Background(), []byte(htmlFixture)); err == nil {
		t.Fatal("HTML must not be accepted as an avatar")
	}
	if _, err := svc.UploadAvatar(context.Background(), []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); err == nil {
		t.Fatal("SVG must not be accepted as an avatar")
	}
	up, err := svc.UploadAvatar(context.Background(), pngBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	if up.ContentType != "image/png" || !strings.HasSuffix(up.Key, ".png") {
		t.Fatalf("a real PNG is re-encoded as PNG, got %s %s", up.ContentType, up.Key)
	}
}

func TestUploadNeverKeepsAnActiveExtension(t *testing.T) {
	svc := newLocalSvc(t)
	if _, err := svc.Upload(context.Background(), "audit-fixture.html", "image/png", []byte(htmlFixture)); err == nil {
		t.Fatal("HTML bytes declared as image/png must be rejected")
	}
	up, err := svc.Upload(context.Background(), "photo.html", "image/png", pngBytes(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(up.Key, ".png") {
		t.Fatalf("the stored extension must follow the type, got %q", up.Key)
	}
	if _, err := svc.Upload(context.Background(), "x.mp3", "text/html", []byte("ID3")); err == nil {
		t.Fatal("a non-media type must be rejected")
	}
}

func TestLicensedUploadStartsUnderProtectedPrefix(t *testing.T) {
	svc := newLocalSvc(t)
	up, err := svc.UploadScoped(context.Background(), "clip.mp4", "video/mp4", []byte("fixture movie"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(up.Key, "protected/") || !strings.Contains(up.URL, "/media/protected/") {
		t.Fatalf("licensed source exposed outside protected storage: %+v", up)
	}
}
