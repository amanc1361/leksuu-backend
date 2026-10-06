// Package rest is the auth module's HTTP delivery layer: handlers, routes, and
// the authentication middleware. No business rules, no user-facing prose.
package rest

import (
	"net/http"
	"strings"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/modules/auth/usecase"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

const bearerPrefix = "Bearer "

// Authenticator validates access tokens and publishes the shared Principal into
// the request context. Exported so other modules can protect their routes with
// the same middleware and read the principal via shared/authn.
type Authenticator struct{ parser usecase.AccessTokenParser }

func NewAuthenticator(parser usecase.AccessTokenParser) *Authenticator {
	return &Authenticator{parser: parser}
}

func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			web.Error(w, r, domain.ErrUnauthenticated)
			return
		}
		claims, err := a.parser.Parse(strings.TrimPrefix(header, bearerPrefix))
		if err != nil {
			web.Error(w, r, domain.ErrTokenInvalid)
			return
		}
		ctx := authn.NewContext(r.Context(), authn.Principal{Subject: claims.Subject, Roles: claims.Roles})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ParseToken validates a raw access token (e.g. from a WebSocket query param,
// since browsers cannot set an Authorization header on a WS handshake) and
// returns the resolved principal. Used by modules that must authenticate
// outside the header-based Require middleware.
func (a *Authenticator) ParseToken(raw string) (authn.Principal, error) {
	claims, err := a.parser.Parse(raw)
	if err != nil {
		return authn.Principal{}, err
	}
	return authn.Principal{Subject: claims.Subject, Roles: claims.Roles}, nil
}

// RequireRole rejects principals lacking the given role.
func (a *Authenticator) RequireRole(role string, next http.Handler) http.Handler {
	return a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := authn.FromContext(r.Context()); ok && p.HasRole(role) {
			next.ServeHTTP(w, r)
			return
		}
		web.Error(w, r, domain.ErrForbidden)
	}))
}
