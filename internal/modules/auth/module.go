// Package auth is the module facade for the authentication bounded context. It
// wires the module's own layers (usecase + security + rest) from injected
// repositories and exposes a minimal surface to the composition root:
// Mount (register routes) and Authenticator (share auth middleware with other
// modules). Adding a feature = a sibling package under internal/modules/.
package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/modules/auth/rest"
	"github.com/leksuu/platform/internal/modules/auth/security"
	"github.com/leksuu/platform/internal/modules/auth/usecase"
)

// Deps are everything the module needs from the outside (repositories + config).
type Deps struct {
	Users       domain.UserRepository
	Tokens      domain.TokenRepository
	JWTSecret   []byte
	AccessTTL   time.Duration
	RefreshTTL  time.Duration
	Clock       func() time.Time
	AdminEmails []string
}

// Module is the wired auth context.
type Module struct {
	handler     *rest.Handler
	authn       *rest.Authenticator
	svc         *usecase.Service
	adminEmails []string
}

// New builds the module graph from its dependencies.
func New(d Deps) *Module {
	jwt := security.NewJWT(d.JWTSecret, d.AccessTTL, d.Clock)
	svc := usecase.NewService(
		d.Users, d.Tokens,
		security.NewPasswordHasher(),
		jwt,
		security.NewRefreshFactory(),
		d.Clock,
		usecase.Config{RefreshTTL: d.RefreshTTL},
		d.AdminEmails,
	)
	return &Module{handler: rest.NewHandler(svc), authn: rest.NewAuthenticator(jwt), svc: svc, adminEmails: d.AdminEmails}
}

// Mount registers the module's routes under the API prefix.
func (m *Module) Mount(mux *http.ServeMux, prefix string) {
	m.handler.Mount(mux, prefix, m.authn)
}

// Authenticator exposes the auth middleware so other modules can protect routes.
func (m *Module) Authenticator() *rest.Authenticator { return m.authn }

// Service exposes the usecase so other modules can resolve user display names.
func (m *Module) Service() *usecase.Service { return m.svc }

// BootstrapAdmins ensures every configured admin email has a usable admin
// account (creating it with the default password if missing). Idempotent.
func (m *Module) BootstrapAdmins(ctx context.Context, password string) {
	for _, e := range m.adminEmails {
		_ = m.svc.EnsureAdmin(ctx, e, password)
	}
}
