package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	URL string `json:"url"`
	Key string `json:"key"`
	// Reused is set when a remote import found the object already stored.
	Reused      bool      `json:"reused,omitempty"`
	ContentType string    `json:"content_type"`
	Width       int       `json:"width,omitempty"`
	Height      int       `json:"height,omitempty"`
	Blur        string    `json:"blur,omitempty"`
	Variants    []Variant `json:"variants,omitempty"`
}

// ArchivedJSON describes an immutable copy of an imported source document.
type ArchivedJSON struct {
	ID           string `json:"id"`
	FileName     string `json:"file_name"`
	StorageKey   string `json:"storage_key"`
	URL          string `json:"url"`
	ContentType  string `json:"content_type"`
	ContentBytes int    `json:"content_bytes"`
	SHA256       string `json:"sha256"`
	// Reused is set when an identical document (same SHA-256 and name) was
	// already archived; nothing new is written.
	Reused bool `json:"reused,omitempty"`
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
	return s.put(ctx, id.New()+ext(filename, contentType), contentType, data)
}

func (s *Service) put(ctx context.Context, key, contentType string, data []byte) (*Uploaded, error) {
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

// Fetch downloads a remote object, returning its bytes and content type.
type Fetch func(ctx context.Context) (data []byte, contentType string, err error)

// ImportRemote copies a remote asset into the store under a key derived from
// its source URL, so importing the same course again reuses what is already
// stored: one HEAD request instead of a download plus an upload.
func (s *Service) ImportRemote(ctx context.Context, sourceURL, filename string, fetch Fetch) (*Uploaded, error) {
	sum := sha256.Sum256([]byte(sourceURL))
	key := "remote/" + hex.EncodeToString(sum[:16]) + remoteExt(filename)
	if lk, ok := s.store.(domain.Lookup); ok {
		if ct, err := lk.Stat(ctx, key); err == nil {
			return &Uploaded{URL: lk.URL(key), Key: key, ContentType: ct, Reused: true}, nil
		}
	}
	data, ct, err := fetch(ctx)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, apperr.New("media.empty", apperr.KindValidation)
	}
	if s.maxBytes > 0 && int64(len(data)) > s.maxBytes {
		return nil, apperr.New("media.too_large", apperr.KindValidation)
	}
	if !allowed(ct) {
		return nil, apperr.New("media.bad_type", apperr.KindValidation)
	}
	return s.put(ctx, key, ct, data)
}

// remoteExts are the extensions a remote key may keep; anything else (CDN
// suffixes like "photo.jpg@jpg", extension-less resize URLs) gets none — the
// stored content type still says what the object is.
var remoteExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true,
	".mp3": true, ".m4a": true, ".ogg": true, ".wav": true, ".mp4": true, ".webm": true,
}

func remoteExt(name string) string {
	if i := strings.IndexAny(name, "@?#"); i >= 0 {
		name = name[:i]
	}
	if e := strings.ToLower(filepath.Ext(name)); remoteExts[e] {
		return e
	}
	return ""
}

// ArchiveJSON stores the exact source used by a course import. The digest in
// the object name makes retries idempotent at the storage layer.
func (s *Service) ArchiveJSON(ctx context.Context, filename string, data []byte) (*ArchivedJSON, error) {
	if len(data) == 0 || (s.maxBytes > 0 && int64(len(data)) > s.maxBytes) {
		return nil, apperr.New("media.too_large", apperr.KindValidation)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if name == "" || name == "." {
		name = "course-import"
	}
	key := "imports/" + digest[:16] + "-" + name + ".json"
	if lk, ok := s.store.(domain.Lookup); ok {
		if _, err := lk.Stat(ctx, key); err == nil {
			return &ArchivedJSON{ID: digest, FileName: filename, StorageKey: key, URL: lk.URL(key), ContentType: "application/json", ContentBytes: len(data), SHA256: digest, Reused: true}, nil
		}
	}
	url, err := s.store.Put(ctx, key, "application/json", data)
	if err != nil {
		return nil, apperr.ErrInternal
	}
	return &ArchivedJSON{ID: digest, FileName: filename, StorageKey: key, URL: url, ContentType: "application/json", ContentBytes: len(data), SHA256: digest}, nil
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
