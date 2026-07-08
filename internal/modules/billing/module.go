// Package billing is the monetization module facade: freemium entitlements +
// subscription purchase via a swappable payment gateway (stub | zarinpal).
package billing

import (
	"net/http"

	"github.com/leksuu/platform/internal/modules/billing/domain"
	"github.com/leksuu/platform/internal/modules/billing/rest"
	"github.com/leksuu/platform/internal/modules/billing/usecase"
)

type Deps struct {
	ProviderKind string // "stub" | "zarinpal"
	MerchantID   string
	Repo         domain.Repository
	Wallet       usecase.Wallet
	Config       usecase.Config
	Guard        rest.Guard
}

type Module struct {
	handler *rest.Handler
	guard   rest.Guard
	svc     *usecase.Service
}

func New(d Deps) *Module {
	var p usecase.PaymentProvider
	switch d.ProviderKind {
	case "zarinpal":
		p = newZarinpal(d.MerchantID)
	default:
		p = stubPaymentProvider{}
	}
	svc := usecase.NewService(d.Repo, p, d.Wallet, d.Config)
	return &Module{handler: rest.NewHandler(svc), guard: d.Guard, svc: svc}
}

func (m *Module) Mount(mux *http.ServeMux, prefix string) { m.handler.Mount(mux, prefix, m.guard) }

// Service exposes the usecase so other modules can enforce quota (e.g. course
// completion calls Consume). Returned as a narrow interface at the call site.
func (m *Module) Service() *usecase.Service { return m.svc }
