package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/leksuu/platform/internal/modules/media/domain"
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
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || strings.EqualFold(u.Hostname(), "localhost") {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	name := in.Filename
	if name == "" {
		name = filepath.Base(u.Path)
	}
	// The host is checked only when the object must actually be downloaded: a
	// URL already copied is reused from storage without any network access, so
	// a re-import cannot lose stored media to a DNS hiccup.
	fetch := func(ctx context.Context) ([]byte, string, error) {
		if err := checkPublicHost(ctx, u.Hostname()); err != nil {
			return nil, "", err
		}
		client := remoteClient(u)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", errFetchFailed
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
				return nil, "", errFetchFailed
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

// errFetchFailed is a transient failure to download a remote object (DNS,
// network, upstream 5xx/429). It maps to a 5xx so import clients retry it,
// unlike a URL that is invalid or points at a private address.
var errFetchFailed = apperr.New("media.fetch_failed", apperr.KindInternal)

// blockedPrefixes are address ranges a remote import may never reach: besides
// what net.IP classifies (private, loopback, link-local, multicast,
// unspecified), carrier-grade NAT, "this network", benchmarking, reserved and
// documentation ranges, and NAT64 / 6to4 / Teredo forms that embed an IPv4.
var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, p := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "2001::/32", "2001:db8::/32", "2002::/16",
	} {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}()

// publicIP reports whether an address may be fetched from (SSRF guard).
func publicIP(ip net.IP) bool {
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

// checkPublicHost refuses hosts that resolve to non-public addresses (SSRF
// guard). A failed lookup is transient.
func checkPublicHost(ctx context.Context, host string) error {
	if strings.EqualFold(host, "localhost") {
		return apperr.ErrBadRequest
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return errFetchFailed
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return apperr.ErrBadRequest
		}
	}
	return nil
}

func publicHost(ctx context.Context, host string) bool {
	return checkPublicHost(ctx, host) == nil
}

// errBlockedAddress aborts a connection to a non-public address.
var errBlockedAddress = errors.New("media: remote address is not public")

// guardedDialer refuses, at connect time, any address that is not public. The
// hostname checks above run before the request; this closes the gap where DNS
// answers differently on the second lookup (DNS rebinding) and covers every
// redirect hop, since each hop dials through it.
var guardedDialer = &net.Dialer{
	Timeout: 15 * time.Second,
	Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return errBlockedAddress
		}
		if ip := net.ParseIP(host); ip == nil || !publicIP(ip) {
			return errBlockedAddress
		}
		return nil
	},
}

// remoteClient builds the HTTP client of a remote import. Connections made
// directly are checked at dial time; when the deployment routes outbound
// traffic through a configured proxy, the proxy resolves and connects, and the
// hostname checks remain the guard.
func remoteClient(u *url.URL) *http.Client {
	checkRedirect := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !publicHost(req.Context(), req.URL.Hostname()) {
			return http.ErrUseLastResponse
		}
		return nil
	}
	if proxy, err := http.ProxyFromEnvironment(&http.Request{URL: u}); err == nil && proxy != nil {
		return &http.Client{Timeout: 45 * time.Second, CheckRedirect: checkRedirect}
	}
	return &http.Client{
		Timeout:       45 * time.Second,
		CheckRedirect: checkRedirect,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           guardedDialer.DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
		},
	}
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
	out := map[string]any{"id": result.ID, "file_name": result.FileName, "storage_key": result.StorageKey, "url": result.URL, "content_type": result.ContentType, "content_bytes": result.ContentBytes, "sha256": result.SHA256, "reused": result.Reused}
	if len(in.Metadata) > 0 {
		out["metadata"] = in.Metadata
	}
	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
	}
	web.JSON(w, status, out)
}

type Handler struct {
	svc            *usecase.Service
	maxBytes       int64
	multipart      domain.MultipartStore
	playbackKey    []byte
	playbackBase   string
	playbackCDN    bool
	playbackAccess func(context.Context, string, string, bool) (PlaybackAsset, error)
	keyOwnership   KeyOwnership
	playlistReader domain.Reader
	playlistDir    string
	// avatarGate answers "may this user upload an avatar?" — wired to the
	// tutor module (approved teachers) after construction.
	avatarGate func(ctx context.Context, userID string) bool
}

// SetAvatarGate wires the late-bound uploader check.
func (h *Handler) SetAvatarGate(f func(ctx context.Context, userID string) bool) { h.avatarGate = f }
func (h *Handler) SetMultipartStore(store domain.MultipartStore)                 { h.multipart = store }

func NewHandler(svc *usecase.Service, maxBytes int64) *Handler {
	return &Handler{svc: svc, maxBytes: maxBytes}
}

// Upload accepts a multipart "file" field and stores it in object storage.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	// The browser routes larger files through signed S3 parts. Bound direct
	// uploads independently of the worker's much larger media size allowance.
	const directUploadMaxBytes int64 = 32 << 20
	limit := directUploadMaxBytes
	if h.maxBytes > 0 && h.maxBytes < limit {
		limit = h.maxBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			web.Error(w, r, apperr.New("media.too_large", apperr.KindValidation))
		} else {
			web.Error(w, r, apperr.ErrBadRequest)
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
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
	if int64(len(data)) > limit {
		web.Error(w, r, apperr.New("media.too_large", apperr.KindValidation))
		return
	}
	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	up, err := h.svc.UploadScoped(r.Context(), hdr.Filename, ct, data, r.FormValue("scope") == "licensed")
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
	file, _, err := r.FormFile("file")
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
	// The declared type and file name are ignored: the bytes decide.
	up, err := h.svc.UploadAvatar(r.Context(), data)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, up)
}
