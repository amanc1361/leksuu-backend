package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leksuu/platform/internal/modules/admin/usecase"
	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

type allowAll struct{}

func (allowAll) RequireRole(_ string, next http.Handler) http.Handler { return next }

// The Busuu importer (frontend admin-api.ts) depends on these routes. A route
// that is not registered answers 404/405 from the mux; anything else means the
// handler ran.
func TestImporterRoutesAreRegistered(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler(usecase.NewService(courserepo.NewMemoryCourseRepo(), nil)).Mount(mux, "/v1", allowAll{})
	routes := []struct{ method, path, body string }{
		{"POST", "/v1/admin/courses", `{}`},
		{"POST", "/v1/admin/courses/x/units", `{}`},
		{"GET", "/v1/admin/courses/x/units", ``},
		{"PATCH", "/v1/admin/courses/x/units/order", `{}`},
		{"GET", "/v1/admin/units/x/lessons", ``},
		{"POST", "/v1/admin/units/x/lessons", `{}`},
		{"PATCH", "/v1/admin/units/x/lessons/order", `{}`},
		{"PATCH", "/v1/admin/lessons/x", `{}`},
		{"PATCH", "/v1/admin/lessons/x/unit", `{"unit_id":"y"}`},
		{"PUT", "/v1/admin/lessons/x/imported-items", `{"items":[]}`},
		{"GET", "/v1/admin/courses/x/translations?locale=fa", ``},
		{"PUT", "/v1/admin/translations", `{}`},
	}
	for _, rt := range routes {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body)))
		if rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "404 page not found") || rec.Code == http.StatusMethodNotAllowed {
			t.Errorf("%s %s is not registered (status %d)", rt.method, rt.path, rec.Code)
		}
	}
}
