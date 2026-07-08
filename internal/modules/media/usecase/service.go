package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	_ "image/gif" // register GIF decoder
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"path/filepath"
	"strconv"
	"strings"

	"github.com/leksuu/platform/internal/modules/media/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

// Uploaded is the result of storing a media object. For images it also carries
// intrinsic dimensions (so the client reserves space and avoids layout shift)
// and a tiny blurred placeholder data URI (LQIP) for an instant blur-up preview.
type Uploaded struct {
	URL         string    `json:"url"`
	Key         string    `json:"key"`
	ContentType string    `json:"content_type"`
	Width       int       `json:"width,omitempty"`
	Height      int       `json:"height,omitempty"`
	Blur        string    `json:"blur,omitempty"`
	Variants    []Variant `json:"variants,omitempty"`
}

// Variant is a resized rendition for responsive delivery (srcset).
type Variant struct {
	URL   string `json:"url"`
	Width int    `json:"width"`
}

type Service struct {
	store    domain.Store
	maxBytes int64
}

func NewService(store domain.Store, maxBytes int64) *Service {
	return &Service{store: store, maxBytes: maxBytes}
}

// Upload validates type/size, generates a stable key, and stores the object.
func (s *Service) Upload(ctx context.Context, filename, contentType string, data []byte) (*Uploaded, error) {
	if len(data) == 0 {
		return nil, apperr.New("media.empty", apperr.KindValidation)
	}
	if s.maxBytes > 0 && int64(len(data)) > s.maxBytes {
		return nil, apperr.New("media.too_large", apperr.KindValidation)
	}
	if !allowed(contentType) {
		return nil, apperr.New("media.bad_type", apperr.KindValidation)
	}
	key := id.New() + ext(filename, contentType)
	url, err := s.store.Put(ctx, key, contentType, data)
	if err != nil {
		return nil, apperr.ErrInternal
	}
	up := &Uploaded{URL: url, Key: key, ContentType: contentType}
	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		enrichImage(up, data)
		s.makeVariants(ctx, up, key, data)
	}
	return up, nil
}

// responsiveWidths are the rendition widths generated on upload. Only widths
// smaller than the original are produced (we never upscale).
var responsiveWidths = []int{480, 800, 1200}

// makeVariants creates smaller JPEG renditions and uploads them, so the client
// can ship a srcset and the browser fetches the smallest size that fits — a real
// bandwidth win on mobile. Best-effort: failures just yield fewer variants.
func (s *Service) makeVariants(ctx context.Context, up *Uploaded, key string, data []byte) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	ow := img.Bounds().Dx()
	base := strings.TrimSuffix(key, filepath.Ext(key))
	for _, w := range responsiveWidths {
		if w >= ow {
			continue // never upscale
		}
		var buf bytes.Buffer
		if jpeg.Encode(&buf, resizeTo(img, w), &jpeg.Options{Quality: 80}) != nil {
			continue
		}
		vkey := base + "_" + strconv.Itoa(w) + ".jpg"
		vurl, err := s.store.Put(ctx, vkey, "image/jpeg", buf.Bytes())
		if err != nil {
			continue
		}
		up.Variants = append(up.Variants, Variant{URL: vurl, Width: w})
	}
	// include the original as the largest candidate in the srcset
	if ow > 0 {
		up.Variants = append(up.Variants, Variant{URL: up.URL, Width: ow})
	}
}

// resizeTo downscales by area-averaging (good quality for shrinking, stdlib-only).
func resizeTo(src image.Image, targetW int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if targetW > sw {
		targetW = sw
	}
	targetH := sh * targetW / sw
	if targetH < 1 {
		targetH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	for dy := 0; dy < targetH; dy++ {
		sy0 := b.Min.Y + dy*sh/targetH
		sy1 := b.Min.Y + (dy+1)*sh/targetH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < targetW; dx++ {
			sx0 := b.Min.X + dx*sw/targetW
			sx1 := b.Min.X + (dx+1)*sw/targetW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var rr, gg, bb, aa, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, bl, a := src.At(sx, sy).RGBA()
					rr += uint64(r)
					gg += uint64(g)
					bb += uint64(bl)
					aa += uint64(a)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			dst.SetRGBA(dx, dy, color.RGBA{
				R: uint8((rr / n) >> 8),
				G: uint8((gg / n) >> 8),
				B: uint8((bb / n) >> 8),
				A: uint8((aa / n) >> 8),
			})
		}
	}
	return dst
}

// enrichImage adds intrinsic dimensions and a low-quality blurred placeholder.
// Best-effort and stdlib-only (jpeg/png/gif); unsupported formats are skipped.
func enrichImage(up *Uploaded, data []byte) {
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		up.Width, up.Height = cfg.Width, cfg.Height
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	thumb := downscale(img, 24)
	var buf bytes.Buffer
	if jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 40}) == nil {
		up.Blur = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	}
}

// downscale is a tiny nearest-neighbour resize (the placeholder is blurred on the
// client, so quality here is irrelevant — only smallness matters).
func downscale(src image.Image, targetW int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return src
	}
	targetH := targetW * h / w
	if targetH < 1 {
		targetH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	for y := 0; y < targetH; y++ {
		sy := b.Min.Y + y*h/targetH
		for x := 0; x < targetW; x++ {
			sx := b.Min.X + x*w/targetW
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func allowed(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "video/")
}

func ext(filename, contentType string) string {
	if e := strings.ToLower(filepath.Ext(filename)); e != "" && len(e) <= 6 {
		return e
	}
	switch {
	case strings.Contains(contentType, "jpeg"):
		return ".jpg"
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	case strings.Contains(contentType, "mpeg"), strings.Contains(contentType, "mp3"):
		return ".mp3"
	case strings.Contains(contentType, "mp4"):
		return ".mp4"
	case strings.Contains(contentType, "webm"):
		return ".webm"
	case strings.Contains(contentType, "ogg"):
		return ".ogg"
	}
	return ".bin"
}
