package rest

import (
	"context"
	"net/http"
)

// Guard wraps a handler with authentication. The auth module's Authenticator
// satisfies it; declaring it here keeps the course module decoupled from auth.
type Guard interface {
	Require(next http.Handler) http.Handler
	// Optional publishes the principal when a valid token is sent and lets
	// anonymous requests through; an invalid token is still rejected (401) so
	// the client refreshes it instead of silently browsing as a guest.
	Optional(next http.Handler) http.Handler
}

// Premium gates film/podcast (media library) behind an active subscription.
type Premium interface {
	RequirePremium(ctx context.Context, userID string) error
}

// Mount registers the course routes. Browse is public (content of locked
// lessons is withheld per viewer); progress requires auth.
func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.HandleFunc("GET "+prefix+"/courses", h.ListCourses)
	mux.HandleFunc("GET "+prefix+"/languages", h.ListLanguages)
	mux.Handle("GET "+prefix+"/courses/{id}", guard.Optional(http.HandlerFunc(h.GetCourse)))
	mux.Handle("GET "+prefix+"/lessons/{id}", guard.Optional(http.HandlerFunc(h.GetLesson)))
	mux.Handle("GET "+prefix+"/media-library", guard.Require(http.HandlerFunc(h.MediaLibrary)))
	mux.Handle("GET "+prefix+"/media-library/items/{id}/progress", guard.Require(http.HandlerFunc(h.GetMediaProgress)))
	mux.Handle("PUT "+prefix+"/media-library/items/{id}/progress", guard.Require(http.HandlerFunc(h.PutMediaProgress)))
	mux.Handle("POST "+prefix+"/media-library/items/{id}/attempts", guard.Require(http.HandlerFunc(h.PostMediaAttempt)))

	mux.Handle("POST "+prefix+"/lessons/{id}/complete", guard.Require(http.HandlerFunc(h.CompleteLesson)))
	mux.Handle("POST "+prefix+"/exercises/{id}/answer", guard.Require(http.HandlerFunc(h.AnswerExercise)))
	mux.Handle("GET "+prefix+"/courses/{id}/progress", guard.Require(http.HandlerFunc(h.CourseProgress)))
	mux.Handle("GET "+prefix+"/practice/quick", guard.Require(http.HandlerFunc(h.QuickPractice)))
}
