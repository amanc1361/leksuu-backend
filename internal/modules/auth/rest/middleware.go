// Package rest is the auth module's HTTP delivery layer: handlers, routes, and
// the authentication middleware. No business rules, no user-facing prose.
package rest

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/modules/auth/usecase"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

const bearerPrefix = "Bearer "

// AccountChecker reports whether a user may still use the API and which roles
// they hold now. The auth service implements it.
type AccountChecker interface {
	AccountState(ctx context.Context, userID string) (active bool, roles []string, err error)
}

// accountTTL bounds how long a looked-up account state is reused. Changes made
// through the admin API invalidate it at once (Forget); the TTL only covers
// changes made outside this process.
const accountTTL = 10 * time.Second

type accountEntry struct {
	active bool
	roles  []string
	at     time.Time
}

// Authenticator validates access tokens and publishes the shared Principal into
// the request context. Exported so other modules can protect their routes with
// the same middleware and read the principal via shared/authn.
//
// Revocation policy: a token's signature and expiry are not enough. With an
// AccountChecker wired, every authenticated request also checks that the
// account is still active and uses its CURRENT roles, so suspending a user or
// removing a role takes effect immediately instead of when the token expires.
type Authenticator struct {
	parser   usecase.AccessTokenParser
	accounts AccountChecker

	mu    sync.Mutex
	cache map[string]accountEntry
	now   func() time.Time
}

func NewAuthenticator(parser usecase.AccessTokenParser) *Authenticator {
	return &Authenticator{parser: parser, cache: map[string]accountEntry{}, now: time.Now}
}

// SetAccountChecker wires the account-state source (the auth service).
func (a *Authenticator) SetAccountChecker(c AccountChecker) { a.accounts = c }

// Forget drops a cached account state, so a status or role change applies to
// the user's very next request.
func (a *Authenticator) Forget(userID string) {
	a.mu.Lock()
	delete(a.cache, userID)
	a.mu.Unlock()
}

func (a *Authenticator) account(ctx context.Context, userID string) (bool, []string, error) {
	now := a.now()
	a.mu.Lock()
	if e, ok := a.cache[userID]; ok && now.Sub(e.at) < accountTTL {
		a.mu.Unlock()
		return e.active, e.roles, nil
	}
	a.mu.Unlock()
	active, roles, err := a.accounts.AccountState(ctx, userID)
	if err != nil {
		return false, nil, err
	}
	a.mu.Lock()
	a.cache[userID] = accountEntry{active: active, roles: roles, at: now}
	a.mu.Unlock()
	return active, roles, nil
}

// principal validates a raw access token and resolves the caller.
func (a *Authenticator) principal(ctx context.Context, raw string) (authn.Principal, error) {
	claims, err := a.parser.Parse(raw)
	if err != nil {
		return authn.Principal{}, domain.ErrTokenInvalid
	}
	p := authn.Principal{Subject: claims.Subject, Roles: claims.Roles}
	if a.accounts == nil {
		return p, nil
	}
	active, roles, err := a.account(ctx, claims.Subject)
	if err != nil {
		return authn.Principal{}, err
	}
	if !active {
		return authn.Principal{}, domain.ErrTokenInvalid
	}
	p.Roles = roles
	return p, nil
}

func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			web.Error(w, r, domain.ErrUnauthenticated)
			return
		}
		p, err := a.principal(r.Context(), strings.TrimPrefix(header, bearerPrefix))
		if err != nil {
			web.Error(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(authn.NewContext(r.Context(), p)))
	})
}

// Optional publishes the principal when the request carries a bearer token and
// passes anonymous requests through unchanged. A token that is sent but invalid
// (expired, revoked account) is rejected with 401, so the client refreshes it
// rather than silently being served the anonymous view.
func (a *Authenticator) Optional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		p, err := a.principal(r.Context(), strings.TrimPrefix(header, bearerPrefix))
		if err != nil {
			web.Error(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(authn.NewContext(r.Context(), p)))
	})
}

// ParseToken validates a raw access token (e.g. from a WebSocket query param,
// since browsers cannot set an Authorization header on a WS handshake) and
// returns the resolved principal. Used by modules that must authenticate
// outside the header-based Require middleware. Applies the same account checks.
func (a *Authenticator) ParseToken(raw string) (authn.Principal, error) {
	return a.principal(context.Background(), raw)
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

// RequireLearner admits only learner accounts: teacher and admin accounts are
// kept out of the learning app's APIs (progress, XP, review, coach…).
func (a *Authenticator) RequireLearner(next http.Handler) http.Handler {
	return a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := authn.FromContext(r.Context()); ok && p.AccountType() == authn.AccountLearner {
			next.ServeHTTP(w, r)
			return
		}
		web.Error(w, r, domain.ErrLearnerOnly)
	}))
}

// LearnerGuard is the guard handed to learning-only modules: their Require
// becomes RequireLearner. Admin routes (RequireRole) are unaffected.
type LearnerGuard struct{ *Authenticator }

// Require admits learner accounts only.
func (g LearnerGuard) Require(next http.Handler) http.Handler { return g.RequireLearner(next) }
