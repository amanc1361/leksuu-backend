package domain

import "github.com/leksuu/platform/internal/shared/apperr"

// Auth sentinel errors. Codes are message-catalog keys (shared/message).
var (
	ErrEmailRequired      = apperr.New("auth.email_required", apperr.KindValidation)
	ErrPasswordRequired   = apperr.New("auth.password_required", apperr.KindValidation)
	ErrRefreshRequired    = apperr.New("auth.refresh_required", apperr.KindValidation)
	ErrEmailTaken         = apperr.New("auth.email_taken", apperr.KindConflict)
	ErrInvalidCredentials = apperr.New("auth.invalid_credentials", apperr.KindUnauthorized)
	ErrUserNotFound       = apperr.New("user.not_found", apperr.KindNotFound)
	ErrTokenInvalid       = apperr.New("auth.token_invalid", apperr.KindUnauthorized)
	ErrTokenReuse         = apperr.New("auth.token_reuse", apperr.KindUnauthorized)
	ErrUnauthenticated    = apperr.New("auth.unauthenticated", apperr.KindUnauthorized)
	ErrForbidden          = apperr.New("auth.forbidden", apperr.KindForbidden)
	ErrInvalidCEFR        = apperr.New("user.invalid_cefr", apperr.KindValidation)
)
