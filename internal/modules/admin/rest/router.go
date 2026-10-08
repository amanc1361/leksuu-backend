package rest

import "net/http"

// Guard restricts routes to a role. The auth Authenticator satisfies it.
type Guard interface {
	RequireRole(role string, next http.Handler) http.Handler
}

const roleAdmin = "admin"

// Mount registers admin routes, each guarded by the admin role.
func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	admin := func(fn http.HandlerFunc) http.Handler { return guard.RequireRole(roleAdmin, fn) }

	mux.Handle("GET "+prefix+"/admin/courses", admin(h.ListCourses))
	mux.Handle("PUT "+prefix+"/admin/access", admin(h.SetAccess))
	mux.Handle("GET "+prefix+"/admin/access", admin(h.AccessRules))
	mux.Handle("POST "+prefix+"/admin/courses", admin(h.CreateCourse))
	mux.Handle("POST "+prefix+"/admin/courses/{id}/publish", admin(h.Publish))
	// units (chapters)
	mux.Handle("GET "+prefix+"/admin/courses/{id}/units", admin(h.ListUnits))
	mux.Handle("POST "+prefix+"/admin/courses/{id}/units", admin(h.CreateUnit))
	mux.Handle("PATCH "+prefix+"/admin/courses/{id}/units/order", admin(h.ReorderUnits))
	mux.Handle("GET "+prefix+"/admin/units/{id}/lessons", admin(h.ListUnitLessons))
	mux.Handle("POST "+prefix+"/admin/units/{id}/lessons", admin(h.CreateUnitLesson))
	mux.Handle("PATCH "+prefix+"/admin/units/{id}/lessons/order", admin(h.ReorderLessons))
	mux.Handle("GET "+prefix+"/admin/courses/{id}/lessons", admin(h.ListLessons))
	mux.Handle("POST "+prefix+"/admin/courses/{id}/lessons", admin(h.CreateLesson))
	mux.Handle("PATCH "+prefix+"/admin/lessons/{id}", admin(h.UpdateLesson))
	mux.Handle("PATCH "+prefix+"/admin/lessons/{id}/unit", admin(h.MoveLesson))
	mux.Handle("GET "+prefix+"/admin/lessons/{id}/items", admin(h.ListItems))
	mux.Handle("POST "+prefix+"/admin/lessons/{id}/items", admin(h.CreateItem))
	mux.Handle("PATCH "+prefix+"/admin/items/{id}", admin(h.UpdateItem))
	mux.Handle("DELETE "+prefix+"/admin/items/{id}", admin(h.DeleteItem))
	mux.Handle("PATCH "+prefix+"/admin/lessons/{id}/items/order", admin(h.ReorderItems))
	mux.Handle("PUT "+prefix+"/admin/lessons/{id}/imported-items", admin(h.ReplaceImportedItems))
	mux.Handle("POST "+prefix+"/admin/items/{id}/exercises", admin(h.CreateExercise))
	// translations of imported explanatory content (grammar tips)
	mux.Handle("GET "+prefix+"/admin/courses/{id}/translations", admin(h.CourseTranslations))
	mux.Handle("PUT "+prefix+"/admin/translations", admin(h.SaveTranslations))
	mux.Handle("POST "+prefix+"/admin/media", admin(h.ImportMedia))
	mux.Handle("PATCH "+prefix+"/admin/media/{id}", admin(h.UpdateMedia))
	mux.Handle("GET "+prefix+"/admin/lesson-templates", admin(h.ListTemplates))
	mux.Handle("GET "+prefix+"/admin/lessons/{id}/context", admin(h.LessonContext))
	// vocabulary bank
	mux.Handle("GET "+prefix+"/admin/vocab", admin(h.ListVocab))
	mux.Handle("POST "+prefix+"/admin/vocab", admin(h.CreateVocab))
	mux.Handle("PATCH "+prefix+"/admin/vocab/{id}", admin(h.UpdateVocab))
	mux.Handle("DELETE "+prefix+"/admin/vocab/{id}", admin(h.DeleteVocab))
}
