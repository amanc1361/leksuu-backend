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

func TestEnrichImageDimsAndBlur(t *testing.T) {
	// build a 40x20 PNG
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 6), G: 80, B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	up := &Uploaded{}
	enrichImage(up, buf.Bytes())
	if up.Width != 40 || up.Height != 20 {
		t.Fatalf("dims = %dx%d, want 40x20", up.Width, up.Height)
	}
	if len(up.Blur) == 0 || up.Blur[:10] != "data:image" {
		t.Fatalf("expected a data-URI blur placeholder, got %q", up.Blur)
	}
}

func TestResizeToNeverUpscalesAndKeepsAspect(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1000, 500))
	out := resizeTo(src, 480)
	if out.Bounds().Dx() != 480 || out.Bounds().Dy() != 240 {
		t.Fatalf("got %dx%d, want 480x240", out.Bounds().Dx(), out.Bounds().Dy())
	}
	// requesting larger than source must not upscale
	out2 := resizeTo(src, 4000)
	if out2.Bounds().Dx() != 1000 {
		t.Fatalf("upscaled to %d, want 1000", out2.Bounds().Dx())
	}
}

func TestImportRemoteReusesStoredObject(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewLocal(dir, "https://leksuu.test")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(st, 1<<20)
	fetches := 0
	fetch := func(context.Context) ([]byte, string, error) {
		fetches++
		return []byte("ID3fake-mp3"), "audio/mpeg", nil
	}
	src := "https://cdn.example.com/audio/clip.mp3"
	first, err := svc.ImportRemote(context.Background(), src, "clip.mp3", fetch)
	if err != nil || first.Reused {
		t.Fatalf("first import: reused=%v err=%v", first != nil && first.Reused, err)
	}
	second, err := svc.ImportRemote(context.Background(), src, "clip.mp3", fetch)
	if err != nil || !second.Reused {
		t.Fatalf("second import should reuse: reused=%v err=%v", second != nil && second.Reused, err)
	}
	if fetches != 1 || first.URL != second.URL {
		t.Fatalf("fetches=%d urls %q vs %q", fetches, first.URL, second.URL)
	}
	if !strings.HasPrefix(first.Key, "remote/") || !strings.HasSuffix(first.Key, ".mp3") {
		t.Fatalf("unexpected key %q", first.Key)
	}
}

func TestRemoteExtDropsCDNSuffixes(t *testing.T) {
	cases := map[string]string{
		"ecc75f16.jpg@jpg":   ".jpg",
		"clip.MP3":           ".mp3",
		"21659016":           "",
		"file.php?x=1.jpg":   "",
		"video.mp4?token=ab": ".mp4",
	}
	for in, want := range cases {
		if got := remoteExt(in); got != want {
			t.Errorf("remoteExt(%q) = %q, want %q", in, got, want)
		}
	}
}
