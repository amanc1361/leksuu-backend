// Package rest exposes the tutor marketplace + booking endpoints. Browsing is
// public; booking, cancelling, reviewing, and joining a room require auth.
package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
	"github.com/leksuu/platform/internal/modules/tutor/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/authn"
	"github.com/leksuu/platform/internal/shared/web"
)

type Handler struct{ svc *usecase.Service }

func NewHandler(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

type Guard interface {
	Require(next http.Handler) http.Handler
	RequireRole(role string, next http.Handler) http.Handler
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string, guard Guard) {
	mux.Handle("GET "+prefix+"/tutors", http.HandlerFunc(h.ListTutors))
	mux.Handle("GET "+prefix+"/tutors/{id}", http.HandlerFunc(h.GetTutor))
	mux.Handle("POST "+prefix+"/bookings", guard.Require(http.HandlerFunc(h.Book)))
	// Paid sessions: pay at the gateway, see class credit, set your price.
	mux.Handle("POST "+prefix+"/bookings/checkout", guard.Require(http.HandlerFunc(h.Checkout)))
	mux.Handle("GET "+prefix+"/bookings/credit", guard.Require(http.HandlerFunc(h.Credit)))
	mux.Handle("PUT "+prefix+"/tutor/me/price", guard.Require(http.HandlerFunc(h.SetMyPrice)))
	mux.Handle("GET "+prefix+"/bookings", guard.Require(http.HandlerFunc(h.MyBookings)))
	mux.Handle("POST "+prefix+"/bookings/{id}/cancel", guard.Require(http.HandlerFunc(h.Cancel)))
	mux.Handle("POST "+prefix+"/bookings/{id}/review", guard.Require(http.HandlerFunc(h.Review)))
	mux.Handle("GET "+prefix+"/bookings/{id}/room", guard.Require(http.HandlerFunc(h.Room)))
	// Tutor-side (the logged-in user must be linked to a tutor profile).
	mux.Handle("POST "+prefix+"/tutor/apply", guard.Require(http.HandlerFunc(h.Apply)))
	mux.Handle("GET "+prefix+"/tutor/application", guard.Require(http.HandlerFunc(h.MyApplication)))
	mux.Handle("GET "+prefix+"/tutor/me", guard.Require(http.HandlerFunc(h.TutorMe)))
	mux.Handle("PATCH "+prefix+"/tutor/me", guard.Require(http.HandlerFunc(h.UpdateMyTutorProfile)))
	mux.Handle("GET "+prefix+"/tutor/slots", guard.Require(http.HandlerFunc(h.MySlots)))
	mux.Handle("POST "+prefix+"/tutor/slots", guard.Require(http.HandlerFunc(h.AddSlots)))
	mux.Handle("DELETE "+prefix+"/tutor/slots/{id}", guard.Require(http.HandlerFunc(h.RemoveSlot)))
	mux.Handle("POST "+prefix+"/tutor/bookings/{id}/cancel", guard.Require(http.HandlerFunc(h.TutorCancelBooking)))
	mux.Handle("GET "+prefix+"/tutor/earnings", guard.Require(http.HandlerFunc(h.Earnings)))
	mux.Handle("GET "+prefix+"/admin/payouts", guard.RequireRole("admin", http.HandlerFunc(h.Payouts)))
	mux.Handle("GET "+prefix+"/admin/users/{id}/sessions", guard.RequireRole("admin", http.HandlerFunc(h.LearnerSessions)))
	mux.Handle("GET "+prefix+"/tutor/bookings", guard.Require(http.HandlerFunc(h.TutorBookings)))
	mux.Handle("GET "+prefix+"/tutor/students", guard.Require(http.HandlerFunc(h.TutorStudents)))
	// Admin onboarding: link a tutor profile to a user account.
	mux.Handle("POST "+prefix+"/admin/tutors", guard.RequireRole("admin", http.HandlerFunc(h.CreateTutor)))
	mux.Handle("POST "+prefix+"/admin/tutors/with-account", guard.RequireRole("admin", http.HandlerFunc(h.CreateTutorWithAccount)))
	mux.Handle("PUT "+prefix+"/admin/tutors/{id}", guard.RequireRole("admin", http.HandlerFunc(h.UpdateTutor)))
	mux.Handle("DELETE "+prefix+"/admin/tutors/{id}", guard.RequireRole("admin", http.HandlerFunc(h.DeleteTutor)))
	mux.Handle("POST "+prefix+"/admin/tutors/{id}/password", guard.RequireRole("admin", http.HandlerFunc(h.SetTutorPassword)))
	mux.Handle("GET "+prefix+"/admin/tutors", guard.RequireRole("admin", http.HandlerFunc(h.AdminListTutors)))
	mux.Handle("POST "+prefix+"/admin/tutors/{id}/approve", guard.RequireRole("admin", http.HandlerFunc(h.ApproveTutor)))
	mux.Handle("POST "+prefix+"/admin/tutors/{id}/reject", guard.RequireRole("admin", http.HandlerFunc(h.RejectTutor)))
	mux.Handle("POST "+prefix+"/admin/tutors/{id}/link", guard.RequireRole("admin", http.HandlerFunc(h.LinkTutor)))
}

func (h *Handler) ListTutors(w http.ResponseWriter, r *http.Request) {
	tutors, err := h.svc.ListTutors(r.Context(), r.URL.Query().Get("lang"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"tutors": tutors})
}

func (h *Handler) GetTutor(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.GetTutor(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, d)
}

func (h *Handler) Book(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		SlotID string `json:"slot_id"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.SlotID == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	b, err := h.svc.Book(r.Context(), p.Subject, in.SlotID)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, bookingView(b))
}

func (h *Handler) MyBookings(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	bs, err := h.svc.MyBookings(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	out := make([]map[string]any, 0, len(bs))
	for i := range bs {
		out = append(out, bookingView(&bs[i]))
	}
	web.JSON(w, http.StatusOK, map[string]any{"bookings": out})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if err := h.svc.Cancel(r.Context(), p.Subject, r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) Review(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	rv, err := h.svc.Review(r.Context(), p.Subject, r.PathValue("id"), in.Rating, in.Comment)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, rv)
}

func (h *Handler) Room(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	info, err := h.svc.Room(r.Context(), p.Subject, r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, info)
}

// TutorMe returns the tutor profile linked to the current user.
func (h *Handler) TutorMe(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	t, err := h.svc.MyTutorProfile(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	if t == nil {
		web.Error(w, r, apperr.New("tutor.not_a_tutor", apperr.KindForbidden))
		return
	}
	web.JSON(w, http.StatusOK, t)
}

// TutorBookings lists the bookings for the tutor linked to the current user.
func (h *Handler) TutorBookings(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	bs, t, err := h.svc.BookingsForTutor(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	if t == nil {
		web.Error(w, r, apperr.New("tutor.not_a_tutor", apperr.KindForbidden))
		return
	}
	out := make([]map[string]any, 0, len(bs))
	for i := range bs {
		m := bookingView(&bs[i].Booking)
		m["learner_name"] = bs[i].LearnerName
		out = append(out, m)
	}
	web.JSON(w, http.StatusOK, map[string]any{"tutor": t, "bookings": out})
}

// TutorStudents returns the roster (one row per student) for the current tutor.
func (h *Handler) TutorStudents(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	students, t, err := h.svc.StudentsForTutor(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	if t == nil {
		web.Error(w, r, apperr.New("tutor.not_a_tutor", apperr.KindForbidden))
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"students": students})
}

// LinkTutor binds a tutor profile to a user account (admin only).
func (h *Handler) LinkTutor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || (in.UserID == "" && in.Email == "") {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	var err error
	if in.Email != "" {
		err = h.svc.LinkUserByEmail(r.Context(), r.PathValue("id"), in.Email)
	} else {
		err = h.svc.LinkUser(r.Context(), r.PathValue("id"), in.UserID)
	}
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// CreateTutor onboards a new tutor profile (admin).
func (h *Handler) CreateTutor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string   `json:"name"`
		Headline   string   `json:"headline"`
		Bio        string   `json:"bio"`
		AvatarURL  string   `json:"avatar_url"`
		Languages  []string `json:"languages"`
		HourlyGems int      `json:"hourly_gems"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	t, err := h.svc.CreateTutor(r.Context(), usecase.CreateTutorInput{
		Name: in.Name, Headline: in.Headline, Bio: in.Bio,
		AvatarURL: in.AvatarURL, Languages: in.Languages, HourlyGems: in.HourlyGems,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, t)
}

// Apply submits a tutor application for the signed-in user (status: pending).
func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Name       string   `json:"name"`
		Headline   string   `json:"headline"`
		Bio        string   `json:"bio"`
		AvatarURL  string   `json:"avatar_url"`
		Languages  []string `json:"languages"`
		HourlyGems int      `json:"hourly_gems"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	t, err := h.svc.Apply(r.Context(), p.Subject, usecase.CreateTutorInput{
		Name: in.Name, Headline: in.Headline, Bio: in.Bio,
		AvatarURL: in.AvatarURL, Languages: in.Languages, HourlyGems: in.HourlyGems,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, t)
}

// MyApplication returns the user's tutor application/profile and its status.
func (h *Handler) MyApplication(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	t, err := h.svc.MyApplication(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"application": t})
}

// ApproveTutor activates a tutor (admin).
func (h *Handler) ApproveTutor(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Approve(r.Context(), r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// RejectTutor declines a tutor application (admin).
func (h *Handler) RejectTutor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if err := h.svc.Reject(r.Context(), r.PathValue("id"), in.Reason); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// AdminListTutors lists all tutors with link status (admin).
func (h *Handler) AdminListTutors(w http.ResponseWriter, r *http.Request) {
	ts, err := h.svc.AdminList(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"tutors": ts})
}

// bookingView adds the derived status so clients don't recompute it.
func bookingView(b *domain.Booking) map[string]any {
	return map[string]any{
		"id": b.ID, "tutor_id": b.TutorID, "learner_id": b.LearnerID, "slot_id": b.SlotID,
		"start_utc": b.StartUTC, "duration_min": b.DurationMin,
		"room_id": b.RoomID, "status": b.EffectiveStatus(time.Now().UTC()),
		"created_at": b.CreatedAt,
	}
}

// Earnings returns the teaching/earnings report for the current tutor.
func (h *Handler) Earnings(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	rep, err := h.svc.EarningsReport(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, rep)
}

// Payouts returns the platform-wide tutor payout liability (admin only).
func (h *Handler) Payouts(w http.ResponseWriter, r *http.Request) {
	sum, err := h.svc.TotalPayout(r.Context())
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, sum)
}

// LearnerSessions returns a user's 1:1 + group session history (admin only).
func (h *Handler) LearnerSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.svc.LearnerSessions(r.Context(), r.PathValue("id"))
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// CreateTutorWithAccount creates a teacher together with a login account (admin).
func (h *Handler) CreateTutorWithAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string   `json:"name"`
		Email      string   `json:"email"`
		Password   string   `json:"password"`
		Headline   string   `json:"headline"`
		Bio        string   `json:"bio"`
		AvatarURL  string   `json:"avatar_url"`
		Languages  []string `json:"languages"`
		HourlyGems int      `json:"hourly_gems"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	t, err := h.svc.CreateTutorWithAccount(r.Context(), usecase.CreateTutorWithAccountInput{
		CreateTutorInput: usecase.CreateTutorInput{
			Name: in.Name, Headline: in.Headline, Bio: in.Bio,
			AvatarURL: in.AvatarURL, Languages: in.Languages, HourlyGems: in.HourlyGems,
		},
		Email: in.Email, Password: in.Password,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, t)
}

// UpdateTutor edits a tutor profile (admin).
func (h *Handler) UpdateTutor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string   `json:"name"`
		Headline   string   `json:"headline"`
		Bio        string   `json:"bio"`
		AvatarURL  string   `json:"avatar_url"`
		Languages  []string `json:"languages"`
		HourlyGems int      `json:"hourly_gems"`
		Status     string   `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	t, err := h.svc.UpdateTutor(r.Context(), r.PathValue("id"), usecase.UpdateTutorInput{
		Name: in.Name, Headline: in.Headline, Bio: in.Bio, AvatarURL: in.AvatarURL,
		Languages: in.Languages, HourlyGems: in.HourlyGems, Status: in.Status,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, t)
}

// DeleteTutor removes a tutor profile and revokes the teacher role (admin).
func (h *Handler) DeleteTutor(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteTutor(r.Context(), r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// SetTutorPassword resets a teacher's login password (admin).
func (h *Handler) SetTutorPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Password == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.SetTutorPassword(r.Context(), r.PathValue("id"), in.Password); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// MySlots lists the calling teacher's upcoming openings.
func (h *Handler) MySlots(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	slots, err := h.svc.MySlots(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"slots": slots})
}

// AddSlots opens new availability: {"starts": [RFC3339...], "duration_min": 30}.
func (h *Handler) AddSlots(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Starts      []time.Time `json:"starts"`
		DurationMin int         `json:"duration_min"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	created, err := h.svc.AddSlots(r.Context(), p.Subject, in.Starts, in.DurationMin)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusCreated, map[string]any{"created": created})
}

// RemoveSlot deletes one unbooked opening owned by the caller.
func (h *Handler) RemoveSlot(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if err := h.svc.RemoveSlot(r.Context(), p.Subject, r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// TutorCancelBooking cancels the caller-teacher's own upcoming class.
func (h *Handler) TutorCancelBooking(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	if err := h.svc.TutorCancel(r.Context(), p.Subject, r.PathValue("id")); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"cancelled": true})
}

// UpdateMyTutorProfile lets the calling teacher edit their public listing.
// PATCH /tutor/me.
func (h *Handler) UpdateMyTutorProfile(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		Headline   string   `json:"headline"`
		Bio        string   `json:"bio"`
		AvatarURL  string   `json:"avatar_url"`
		Languages  []string `json:"languages"`
		HourlyGems int      `json:"hourly_gems"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	t, err := h.svc.UpdateMyProfile(r.Context(), p.Subject, usecase.UpdateMyProfileInput{
		Headline: in.Headline, Bio: in.Bio, AvatarURL: in.AvatarURL, Languages: in.Languages, HourlyGems: in.HourlyGems,
	})
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, t)
}

// Checkout holds the slot and returns the gateway URL to pay for it.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		SlotID string `json:"slot_id"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.SlotID == "" {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	url, err := h.svc.CheckoutSession(r.Context(), p.Subject, in.SlotID)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"pay_url": url})
}

// Credit returns the learner's class credit (refunds), in Toman.
func (h *Handler) Credit(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	bal, err := h.svc.Credit(r.Context(), p.Subject)
	if err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"balance_toman": bal})
}

// SetMyPrice sets the signed-in teacher's price per session (Toman).
func (h *Handler) SetMyPrice(w http.ResponseWriter, r *http.Request) {
	p, ok := authn.FromContext(r.Context())
	if !ok {
		web.Error(w, r, apperr.ErrInternal)
		return
	}
	var in struct {
		PriceToman int `json:"price_toman"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		web.Error(w, r, apperr.ErrBadRequest)
		return
	}
	if err := h.svc.SetMyPrice(r.Context(), p.Subject, in.PriceToman); err != nil {
		web.Error(w, r, err)
		return
	}
	web.JSON(w, http.StatusOK, map[string]any{"price_toman": in.PriceToman})
}
