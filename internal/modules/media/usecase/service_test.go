package usecase

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
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
