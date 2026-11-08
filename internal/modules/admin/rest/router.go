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
	// target languages (add / enable / disable / order)
	mux.Handle("GET "+prefix+"/admin/languages", admin(h.ListLanguages))
	mux.Handle("POST "+prefix+"/admin/languages", admin(h.CreateLanguage))
	mux.Handle("PATCH "+prefix+"/admin/languages/{code}", admin(h.UpdateLanguage))
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
	mux.Handle("GET "+prefix+"/admin/media", admin(h.ListMedia))
	mux.Handle("GET "+prefix+"/admin/processing/jobs", admin(h.ListMediaJobs))
	mux.Handle("POST "+prefix+"/admin/processing/jobs", admin(h.EnqueueMediaJob))
	mux.Handle("GET "+prefix+"/admin/processing/jobs/{id}", admin(h.GetMediaJob))
	mux.Handle("POST "+prefix+"/admin/processing/jobs/{id}/cancel", admin(h.CancelMediaJob))
	mux.Handle("POST "+prefix+"/admin/processing/jobs/{id}/retry", admin(h.RetryMediaJob))
	mux.Handle("POST "+prefix+"/admin/processing/jobs/{id}/approve", admin(h.ApproveMediaJob))
	mux.Handle("PATCH "+prefix+"/admin/processing/jobs/{id}/cue", admin(h.ReviseMediaJobCue))
	mux.Handle("PATCH "+prefix+"/admin/media/{id}", admin(h.UpdateMedia))
	mux.Handle("PATCH "+prefix+"/admin/media/{id}/cues", admin(h.ReviewMediaCue))
	mux.Handle("PATCH "+prefix+"/admin/media/{id}/publication", admin(h.SetMediaPublication))
	mux.Handle("POST "+prefix+"/admin/media/{id}/publication-review", admin(h.ReviewMediaPublication))
	mux.Handle("POST "+prefix+"/admin/stories/import", admin(h.ImportStoryArchive))
	mux.Handle("PUT "+prefix+"/admin/stories/{sourceID}/timing", admin(h.BackfillStoryTiming))
	mux.Handle("PATCH "+prefix+"/admin/stories/{sourceID}/timing", admin(h.ReviseStoryTiming))
	mux.Handle("GET "+prefix+"/admin/stories/timing-exceptions", admin(h.ListTimingExceptions))
	mux.Handle("PUT "+prefix+"/admin/stories/timing-exceptions", admin(h.IngestTimingExceptions))
	mux.Handle("PUT "+prefix+"/admin/stories/{sourceID}/translations", admin(h.BackfillStoryTranslations))
	mux.Handle("PATCH "+prefix+"/admin/stories/{sourceID}/translations", admin(h.ReviseStoryTranslation))
	mux.Handle("POST "+prefix+"/admin/stories/{sourceID}/translations/approve", admin(h.ApproveStoryTranslations))
	mux.Handle("POST "+prefix+"/admin/stories/{sourceID}/translations/decision", admin(h.DecideStoryTranslation))
	mux.Handle("PATCH "+prefix+"/admin/media/{sourceID}/translations", admin(h.ReviseStoryTranslation))
	mux.Handle("POST "+prefix+"/admin/media/{sourceID}/translations/decision", admin(h.DecideStoryTranslation))
	mux.Handle("POST "+prefix+"/admin/stories/bulk", admin(h.BulkStories))
	mux.Handle("POST "+prefix+"/admin/media/{id}/cover-generations", admin(h.GenerateStoryCover))
	mux.Handle("POST "+prefix+"/admin/media/{id}/cover-generations/approve", admin(h.ApproveStoryCover))
	mux.Handle("POST "+prefix+"/admin/media/{id}/cover-generations/restore", admin(h.RestoreStoryCover))
	mux.Handle("GET "+prefix+"/admin/lesson-templates", admin(h.ListTemplates))
	mux.Handle("GET "+prefix+"/admin/lessons/{id}/context", admin(h.LessonContext))
	// vocabulary bank
	mux.Handle("GET "+prefix+"/admin/vocab", admin(h.ListVocab))
	mux.Handle("POST "+prefix+"/admin/vocab", admin(h.CreateVocab))
	mux.Handle("POST "+prefix+"/admin/vocab/import", admin(h.ImportVocabulary))
	mux.Handle("PATCH "+prefix+"/admin/vocab/{id}", admin(h.UpdateVocab))
	mux.Handle("DELETE "+prefix+"/admin/vocab/{id}", admin(h.DeleteVocab))
}
