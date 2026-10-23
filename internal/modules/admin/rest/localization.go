package rest

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/shared/authn"
)

// LocalizationProxy forwards only after the application's real admin-role
// authorization. The internal token and end-user JWT never reach AI providers.
func LocalizationProxy(baseURL, token, prefix string) http.Handler {
	target, err := url.Parse(baseURL)
	if err != nil || target.Host == "" || token == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"localization_not_configured"}}`))
		})
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = strings.TrimRight(target.Path, "/") + "/" + strings.TrimPrefix(pr.In.URL.Path, prefix)
			pr.Out.URL.RawPath = ""
			pr.Out.Header.Set("Authorization", "Bearer "+token)
			principal, _ := authn.FromContext(pr.In.Context())
			pr.Out.Header.Set("X-Localization-Actor", principal.Subject)
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("X-Api-Key")
		},
		Transport: &http.Transport{ResponseHeaderTimeout: 40 * time.Second, MaxIdleConns: 10, IdleConnTimeout: 60 * time.Second},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"code":"localization_unavailable"}}`))
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 262144)
		proxy.ServeHTTP(w, r)
	})
}
