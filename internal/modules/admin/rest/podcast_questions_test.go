package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leksuu/platform/internal/modules/admin/usecase"
	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
)

type podcastTestGuard struct{ t *testing.T }

func (g podcastTestGuard) RequireRole(role string, next http.Handler) http.Handler {
	if role != "admin" {
		g.t.Fatalf("unexpected route role: %s", role)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Role") != "admin" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func TestPodcastQuestionsRouteRequiresAdminAndStrictBody(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler(usecase.NewService(courserepo.NewMemoryCourseRepo(), nil)).Mount(mux, "/api/v1", podcastTestGuard{t})
	for _, tc := range []struct {
		name, role, body string
		status           int
	}{
		{"anonymous", "", `{}`, http.StatusForbidden},
		{"learner", "learner", `{}`, http.StatusForbidden},
		{"unknown field", "admin", `{"questions":[],"expected_questions":null,"published":true}`, http.StatusBadRequest},
		{"multiple objects", "admin", `{"questions":[],"expected_questions":null}{}`, http.StatusBadRequest},
		{"missing expected snapshot", "admin", `{"questions":[]}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/media/fixture/listening-questions", strings.NewReader(tc.body))
			r.Header.Set("X-Test-Role", tc.role)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d", w.Code, tc.status)
			}
		})
	}
}
