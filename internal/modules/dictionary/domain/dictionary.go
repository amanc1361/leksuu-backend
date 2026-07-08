// Package domain defines the external-dictionary provider model: admin-registered
// HTTP endpoints that take a word and return its meaning (translation/definition).
package domain

import (
	"context"

	"github.com/leksuu/platform/internal/shared/apperr"
)

// Provider is a configurable external lookup API. URL/BodyTemplate may contain
// the literal placeholder {word}, replaced with the queried term at call time.
type Provider struct {
	ID           string
	Name         string
	URL          string
	Method       string
	HeaderName   string
	HeaderValue  string
	BodyTemplate string
	ResponsePath string
	Enabled      bool
	Position     int
}

var (
	ErrProviderNotFound = apperr.New("dictionary.provider_not_found", apperr.KindNotFound)
	ErrNoResult         = apperr.New("dictionary.no_result", apperr.KindNotFound)
)

type Repository interface {
	List(ctx context.Context) ([]Provider, error)
	Get(ctx context.Context, id string) (*Provider, error)
	Create(ctx context.Context, p *Provider) error
	Update(ctx context.Context, p *Provider) error
	Delete(ctx context.Context, id string) error
	Enabled(ctx context.Context) ([]Provider, error)
}
