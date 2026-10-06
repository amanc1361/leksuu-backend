package rest

import (
	"context"
	"net/http"
)

// Guard wraps a handler with authentication. The auth module's Authenticator
// satisfies it; declaring it here keeps the course module decoupled from auth.
type Guard interface {
	Require(next http.Handler) http.Handler
}

// Premium gates film/podcast (media library) behind an active subscription.
type Premium interface {
	RequirePremium(ctx context.Context, userID string) error
}

// Mount registers the course routes. Browse is public; progress requires auth.
func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.HandleFunc("GET "+prefix+"/courses", h.ListCourses)
	mux.HandleFunc("GET "+prefix+"/courses/{id}", h.GetCourse)
	mux.HandleFunc("GET "+prefix+"/lessons/{id}", h.GetLesson)
	mux.Handle("GET "+prefix+"/media-library", guard.Require(http.HandlerFunc(h.MediaLibrary)))

	mux.Handle("POST "+prefix+"/lessons/{id}/complete", guard.Require(http.HandlerFunc(h.CompleteLesson)))
	mux.Handle("POST "+prefix+"/exercises/{id}/answer", guard.Require(http.HandlerFunc(h.AnswerExercise)))
	mux.Handle("GET "+prefix+"/courses/{id}/progress", guard.Require(http.HandlerFunc(h.CourseProgress)))
	mux.Handle("GET "+prefix+"/practice/quick", guard.Require(http.HandlerFunc(h.QuickPractice)))
}
