// Package web holds generic HTTP helpers shared by every module's delivery
// layer: JSON encoding and the single error->status+localized-message mapping.
package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/message"
)

// JSON writes a success payload.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type errorBody struct {
	Error errorDetail `json:"error"`
}
type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error maps any error to (status, code, localized message). Unknown errors are
// treated as internal so details never leak.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		ae = apperr.ErrInternal
	}
	locale := message.Negotiate(r.Header.Get("Accept-Language"))
	JSON(w, statusFor(ae.Kind), errorBody{Error: errorDetail{
		Code:    ae.Code,
		Message: message.Localize(locale, ae.Code),
	}})
}

func statusFor(kind apperr.Kind) int {
	switch kind {
	case apperr.KindValidation:
		return http.StatusBadRequest
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
