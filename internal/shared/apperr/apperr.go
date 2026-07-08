// Package apperr defines the application's error vocabulary. It is part of the
// shared kernel: every module builds its domain errors from this type, and the
// delivery layer maps Kind -> transport status without knowing any module.
package apperr

// Kind classifies an error so transports (HTTP, gRPC, …) can translate it to a
// status code without the domain depending on any transport.
type Kind int

const (
	KindInternal Kind = iota
	KindValidation
	KindNotFound
	KindConflict
	KindUnauthorized
	KindForbidden
)

// Error carries a stable machine code (a message-catalog key) and a Kind.
// The code is never user-facing prose — that lives in shared/message.
type Error struct {
	Code string
	Kind Kind
}

func (e *Error) Error() string { return e.Code }

// New builds a sentinel error. Modules call this in their domain package.
func New(code string, kind Kind) *Error { return &Error{Code: code, Kind: kind} }

// Generic, cross-module errors.
var (
	ErrInternal   = New("common.internal", KindInternal)
	ErrBadRequest = New("common.bad_request", KindValidation)
)
