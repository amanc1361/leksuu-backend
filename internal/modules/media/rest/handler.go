package rest

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/media/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type remoteInput struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

// ImportRemote copies a public HTTP(S) asset into the configured object store.
// Private/link-local destinations are rejected to keep this admin tool from
// becoming an SSRF path into the deployment network.
func (h *Handler) ImportRemote(w http.ResponseWriter, r *http.Request) {
	var in remoteInput
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || !publicHost(r.Context(), u.Hostname()) {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	name := in.Filename
	if name == "" {
		name = filepath.Base(u.Path)
	}
	fetch := func(ctx context.Context) ([]byte, string, error) {
		client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !publicHost(req.Context(), req.URL.Hostname()) {
				return http.ErrUseLastResponse
			}
			return nil
		}}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if resp != nil {
				resp.Body.Close()
			}
			return nil, "", apperr.ErrBadRequest
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, h.maxBytes+1))
		if err != nil || int64(len(data)) > h.maxBytes {
			return nil, "", apperr.New("media.too_large", apperr.KindValidation)
		}
		ct := in.ContentType
		if ct == "" {
			ct = strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
		}
		if ct == "" {
			ct = http.DetectContentType(data)
		}
		if ct == "application/octet-stream" || ct == "binary/octet-stream" {
			if inferred := mime.TypeByExtension(remoteExtOf(name)); inferred != "" {
				ct = strings.TrimSpace(strings.Split(inferred, ";")[0])
			} else {
				ct = http.DetectContentType(data)
			}
		}
		return data, ct, nil
	}
	up, err := h.svc.ImportRemote(r.Context(), u.String(), name, fetch)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	status := http.StatusCreated
	if up.Reused {
		status = http.StatusOK
	}
	web.JSON(w, status, up)
}

// remoteExtOf is the file extension of a remote name, ignoring CDN suffixes
// such as "photo.jpg@jpg".
func remoteExtOf(name string) string {
	if i := strings.IndexAny(name, "@?#"); i >= 0 {
		name = name[:i]
	}
	return filepath.Ext(name)
}

func publicHost(ctx context.Context, host string) bool {
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return false
		}
	}
	return true
}

func (h *Handler) ArchiveCourseImport(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FileName string          `json:"file_name"`
		RawJSON  string          `json:"raw_json"`
		Metadata json.RawMessage `json:"metadata"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, h.maxBytes+(1<<20))).Decode(&in) != nil || in.RawJSON == "" || !json.Valid([]byte(in.RawJSON)) {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	result, err := h.svc.ArchiveJSON(r.Context(), in.FileName, []byte(in.RawJSON))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := map[string]any{"id": result.ID, "file_name": result.FileName, "storage_key": result.StorageKey, "url": result.URL, "content_type": result.ContentType, "content_bytes": result.ContentBytes, "sha256": result.SHA256}
	if len(in.Metadata) > 0 {
		out["metadata"] = in.Metadata
	}
	web.JSON(w, http.StatusCreated, out)
}

type Handler struct {
	svc      *usecase.Service
	maxBytes int64
	// avatarGate answers "may this user upload an avatar?" — wired to the
	// tutor module (approved teachers) after construction.
	avatarGate func(ctx context.Context, userID string) bool
}

// SetAvatarGate wires the late-bound uploader check.
func (h *Handler) SetAvatarGate(f func(ctx context.Context, userID string) bool) { h.avatarGate = f }

func NewHandler(svc *usecase.Service, maxBytes int64) *Handler {
	return &Handler{svc: svc, maxBytes: maxBytes}
}

// Upload accepts a multipart "file" field and stores it in object storage.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	up, err := h.svc.Upload(r.Context(), hdr.Filename, ct, data)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, up)
}

// avatarMaxBytes keeps avatars small (they render at ≤128px anywhere).
const avatarMaxBytes = 2 << 20

// UploadAvatar accepts an image for the caller's public listing. Gated to
// approved teachers via the avatar gate; images only, 2MB cap.
func (h *Handler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if h.avatarGate == nil || !h.avatarGate(r.Context(), p.Subject) {
		web.Error(w, r, apperr.New("media.not_allowed", apperr.KindForbidden))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, avatarMaxBytes+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if len(data) > avatarMaxBytes {
		web.Error(w, r, apperr.New("media.too_large", apperr.KindValidation))
		return
	}
	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(ct), "image/") {
		web.Error(w, r, apperr.New("media.bad_type", apperr.KindValidation))
		return
	}
	up, err := h.svc.Upload(r.Context(), hdr.Filename, ct, data)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, up)
}
