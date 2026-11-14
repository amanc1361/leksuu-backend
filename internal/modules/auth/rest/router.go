package rest

import "net/http"

// Mount registers the auth routes under the given prefix on the mux. Protected
// routes use the provided Authenticator.
func (h *Handler) Mount(mux *http.ServeMux, prefix string, authn *Authenticator) {
	mux.HandleFunc("POST "+prefix+"/auth/register", h.Register)
	mux.HandleFunc("POST "+prefix+"/auth/login", h.Login)
	mux.HandleFunc("POST "+prefix+"/auth/refresh", h.Refresh)
	mux.HandleFunc("POST "+prefix+"/auth/logout", h.Logout)
	mux.Handle("GET "+prefix+"/me", authn.Require(http.HandlerFunc(h.Me)))
	mux.Handle("PATCH "+prefix+"/me", authn.Require(http.HandlerFunc(h.UpdateProfile)))
	mux.Handle("GET "+prefix+"/me/onboarding", authn.Require(http.HandlerFunc(h.GetOnboarding)))
	mux.Handle("PUT "+prefix+"/me/onboarding", authn.Require(http.HandlerFunc(h.CompleteOnboarding)))
	mux.Handle("GET "+prefix+"/me/learning-records/{kind}", authn.Require(http.HandlerFunc(h.LearningRecords)))
	mux.Handle("POST "+prefix+"/me/learning-records/{kind}", authn.Require(http.HandlerFunc(h.SaveLearningRecord)))
	mux.Handle("POST "+prefix+"/me/password", authn.Require(http.HandlerFunc(h.ChangePassword)))
	mux.Handle("GET "+prefix+"/admin/users", authn.RequireRole("admin", http.HandlerFunc(h.ListUsers)))
	mux.Handle("GET "+prefix+"/admin/users/{id}", authn.RequireRole("admin", http.HandlerFunc(h.UserDetail)))
	mux.Handle("PUT "+prefix+"/admin/users/{id}/roles", authn.RequireRole("admin", http.HandlerFunc(h.SetUserRole)))
	mux.Handle("PUT "+prefix+"/admin/users/{id}/account-type", authn.RequireRole("admin", http.HandlerFunc(h.SetAccountType)))
	mux.Handle("POST "+prefix+"/admin/users/bulk-roles", authn.RequireRole("admin", http.HandlerFunc(h.BulkSetUserRole)))
	mux.Handle("PUT "+prefix+"/admin/users/{id}/status", authn.RequireRole("admin", http.HandlerFunc(h.SetUserStatus)))
}
