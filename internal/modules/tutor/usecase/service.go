// Package usecase holds tutor-marketplace logic: browsing tutors, booking a
// slot, listing/cancelling bookings, reviewing a finished session, and issuing
// a video-room join descriptor. All times are UTC; the delivery layer localizes.
package usecase

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/reminder"
	"github.com/leksuu/platform/internal/modules/tutor/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

// UserDirectory resolves user IDs to display names (implemented by the auth
// module) so the tutor portal can show real student names.
type UserDirectory interface {
	DisplayNames(ctx context.Context, ids []string) (map[string]string, error)
	UserByEmail(ctx context.Context, email string) (id, displayName string, ok bool, err error)
	// CreateUser provisions a new account (used to create a teacher's login).
	CreateUser(ctx context.Context, email, displayName, password string) (id string, err error)
	// AdminSetPassword resets a user's password (teacher password management).
	AdminSetPassword(ctx context.Context, userID, password string) error
}

// Notifier emits in-app notifications (optional).
type Notifier interface {
	Notify(ctx context.Context, userID, ntype string, data map[string]string, link string) error
}

// ClassAccess gates booking behind an active class subscription (optional).
type ClassAccess interface {
	RequireClass(ctx context.Context, userID string) error
}

// JoinWindow supplies the live-room join window in minutes (optional).
type JoinWindow interface {
	RoomJoinWindowMin(ctx context.Context) int
}

// PayoutRate supplies the per-hour tutor payout rate in Toman (optional).
type PayoutRate interface {
	TutorPayoutPerHourToman(ctx context.Context) int
}

// GroupSession is one group class a tutor scheduled, for the earnings report.
type GroupSession struct {
	StartUTC    time.Time
	DurationMin int
	Title       string
	Attendees   int
	Completed   bool
}

// GroupSessionSource supplies group-class teaching sessions (optional).
type GroupSessionSource interface {
	TutorGroupSessions(ctx context.Context, tutorUserID string) ([]GroupSession, error)
	AllGroupSessions(ctx context.Context) ([]GroupSession, error)
	StudentGroupSessions(ctx context.Context, studentUserID string) ([]GroupSession, error)
}

// roomJoinState reports whether a session room is open relative to its window:
// it opens windowMin before the start and closes at the end.
func roomJoinState(now, start time.Time, durationMin, windowMin int) (state string, opensAt, end time.Time) {
	if windowMin <= 0 {
		windowMin = 10
	}
	opensAt = start.Add(-time.Duration(windowMin) * time.Minute)
	end = start.Add(time.Duration(durationMin) * time.Minute)
	switch {
	case now.Before(opensAt):
		return "early", opensAt, end
	case now.After(end):
		return "finished", opensAt, end
	default:
		return "open", opensAt, end
	}
}

// RoleAssigner grants account roles (a linked tutor becomes a "teacher").
type RoleAssigner interface {
	AssignRole(ctx context.Context, userID, role string) error
	RemoveRole(ctx context.Context, userID, role string) error
}

// roleTeacher mirrors auth.domain.RoleTeacher.
const roleTeacher = "teacher"

type Service struct {
	repo        domain.Repository
	clock       func() time.Time
	users       UserDirectory
	notif       Notifier
	classAccess ClassAccess
	joinWindow  JoinWindow
	payout      PayoutRate
	groupSess   GroupSessionSource
	roles       RoleAssigner
}

// SetDirectory wires the user-name resolver (optional; nil-safe).
func (s *Service) SetDirectory(u UserDirectory) { s.users = u }

// SetNotifier wires the notification emitter (optional; nil-safe).
func (s *Service) SetNotifier(n Notifier) { s.notif = n }

// SetClassAccess wires the class-subscription gate for booking (optional).
func (s *Service) SetClassAccess(c ClassAccess) { s.classAccess = c }

// SetJoinWindow wires the room join-window source (optional; default 10 min).
func (s *Service) SetJoinWindow(j JoinWindow) { s.joinWindow = j }

// SetPayoutRate wires the tutor payout-rate source (optional).
func (s *Service) SetPayoutRate(p PayoutRate) { s.payout = p }

// SetGroupSessions wires the tutor's group-class session source (optional).
func (s *Service) SetGroupSessions(g GroupSessionSource) { s.groupSess = g }

func (s *Service) payoutPerHour(ctx context.Context) int {
	if s.payout != nil {
		if r := s.payout.TutorPayoutPerHourToman(ctx); r > 0 {
			return r
		}
	}
	return 0
}

func (s *Service) windowMin(ctx context.Context) int {
	if s.joinWindow != nil {
		if m := s.joinWindow.RoomJoinWindowMin(ctx); m > 0 {
			return m
		}
	}
	return 10
}

// SetRoles wires the role assigner used to grant the teacher role on linking.
func (s *Service) SetRoles(r RoleAssigner) { s.roles = r }

// notifyTutor emits a notification to the user account behind a tutor profile.
func (s *Service) notifyTutor(ctx context.Context, tutorID, ntype string, data map[string]string) {
	if s.notif == nil {
		return
	}
	t, _ := s.repo.Tutor(ctx, tutorID)
	if t != nil && t.UserID != "" {
		_ = s.notif.Notify(ctx, t.UserID, ntype, data, "/teach")
	}
}

func (s *Service) learnerName(ctx context.Context, learnerID string) string {
	return s.names(ctx, []string{learnerID})[learnerID]
}

// DueReminders builds class-reminder notifications for confirmed 1:1 bookings
// starting within [from, to]. Satisfies reminder.Source. Each participant sees
// the other party's name.
func (s *Service) DueReminders(ctx context.Context, from, to time.Time) ([]reminder.Reminder, error) {
	bs, err := s.repo.BookingsStarting(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]reminder.Reminder, 0, len(bs))
	for _, b := range bs {
		t, _ := s.repo.Tutor(ctx, b.TutorID)
		tutorUID, tutorName := "", ""
		if t != nil {
			tutorUID, tutorName = t.UserID, t.Name
		}
		learnerName := s.learnerName(ctx, b.LearnerID)
		when := b.StartUTC.Format(time.RFC3339)
		targets := []reminder.Target{
			{UserID: b.LearnerID, Data: map[string]string{"name": tutorName, "when": when}},
		}
		if tutorUID != "" {
			targets = append(targets, reminder.Target{UserID: tutorUID, Data: map[string]string{"name": learnerName, "when": when}})
		}
		out = append(out, reminder.Reminder{Key: b.ID, Type: reminder.TypeClassReminder, Link: "/room/" + b.ID, Targets: targets})
	}
	return out, nil
}

func (s *Service) names(ctx context.Context, ids []string) map[string]string {
	if s.users == nil {
		return map[string]string{}
	}
	m, _ := s.users.DisplayNames(ctx, ids)
	if m == nil {
		return map[string]string{}
	}
	return m
}

func NewService(repo domain.Repository, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, clock: clock}
}

func (s *Service) now() time.Time { return s.clock().UTC() }

func (s *Service) ListTutors(ctx context.Context, lang string) ([]domain.Tutor, error) {
	all, err := s.repo.ListTutors(ctx, strings.ToLower(strings.TrimSpace(lang)))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Tutor, 0, len(all))
	for _, t := range all {
		if t.Status == domain.StatusApproved {
			out = append(out, t)
		}
	}
	return out, nil
}

// TutorDetail bundles a tutor with their upcoming free slots.
type TutorDetail struct {
	Tutor        domain.Tutor  `json:"tutor"`
	Availability []domain.Slot `json:"availability"`
}

func (s *Service) GetTutor(ctx context.Context, tutorID string) (*TutorDetail, error) {
	t, err := s.repo.Tutor(ctx, tutorID)
	if err != nil {
		return nil, err
	}
	if t != nil && t.Status != domain.StatusApproved {
		t = nil // pending/rejected tutors are not publicly viewable
	}
	if t == nil {
		return nil, apperr.New("tutor.not_found", apperr.KindNotFound)
	}
	slots, err := s.repo.Availability(ctx, tutorID, s.now())
	if err != nil {
		return nil, err
	}
	return &TutorDetail{Tutor: *t, Availability: slots}, nil
}

// Book reserves a future, unbooked slot for the learner and opens a room.
func (s *Service) Book(ctx context.Context, learnerID, slotID string) (*domain.Booking, error) {
	slot, err := s.repo.Slot(ctx, slotID)
	if err != nil {
		return nil, err
	}
	if slot == nil {
		return nil, apperr.New("tutor.slot_not_found", apperr.KindNotFound)
	}
	if slot.Booked {
		return nil, apperr.New("tutor.slot_taken", apperr.KindConflict)
	}
	if !slot.StartUTC.After(s.now()) {
		return nil, apperr.New("tutor.slot_past", apperr.KindValidation)
	}

	// Live-class access is gated by an active class subscription (no gems).
	if s.classAccess != nil {
		if err := s.classAccess.RequireClass(ctx, learnerID); err != nil {
			return nil, err
		}
	}

	if err := s.repo.SetSlotBooked(ctx, slot.ID, true); err != nil {
		return nil, err
	}
	b := &domain.Booking{
		ID:          id.New(),
		TutorID:     slot.TutorID,
		LearnerID:   learnerID,
		SlotID:      slot.ID,
		StartUTC:    slot.StartUTC,
		DurationMin: slot.DurationMin,
		Status:      domain.StatusConfirmed,
		RoomID:      "room_" + id.New(),
		CreatedAt:   s.now(),
	}
	if err := s.repo.CreateBooking(ctx, b); err != nil {
		_ = s.repo.SetSlotBooked(ctx, slot.ID, false) // best-effort release
		return nil, err
	}
	s.notifyTutor(ctx, b.TutorID, domain.NotifBookingNew, map[string]string{
		"learner_name": s.learnerName(ctx, learnerID), "when": b.StartUTC.Format(time.RFC3339),
	})
	return b, nil
}

// SessionTimes returns the start times of every non-cancelled 1:1 session, so
// the admin's platform-wide class stats can count live 1:1 classes alongside
// group classes (one consistent definition of "class").
func (s *Service) SessionTimes(ctx context.Context) ([]time.Time, error) {
	bs, err := s.repo.AllBookings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, len(bs))
	for _, b := range bs {
		if b.Status == domain.StatusCancelled {
			continue
		}
		out = append(out, b.StartUTC)
	}
	return out, nil
}

func (s *Service) MyBookings(ctx context.Context, learnerID string) ([]domain.Booking, error) {
	return s.repo.BookingsByLearner(ctx, learnerID)
}

// Cancel releases a future booking owned by the learner and frees its slot.
func (s *Service) Cancel(ctx context.Context, learnerID, bookingID string) error {
	b, err := s.repo.Booking(ctx, bookingID)
	if err != nil {
		return err
	}
	if b == nil || b.LearnerID != learnerID {
		return apperr.New("tutor.booking_not_found", apperr.KindNotFound)
	}
	if b.Status == domain.StatusCancelled {
		return nil
	}
	if !b.StartUTC.After(s.now()) {
		return apperr.New("tutor.too_late_to_cancel", apperr.KindValidation)
	}
	if err := s.repo.UpdateBookingStatus(ctx, b.ID, domain.StatusCancelled); err != nil {
		return err
	}
	s.notifyTutor(ctx, b.TutorID, domain.NotifBookingCancelled, map[string]string{
		"learner_name": s.learnerName(ctx, learnerID), "when": b.StartUTC.Format(time.RFC3339),
	})
	return s.repo.SetSlotBooked(ctx, b.SlotID, false)
}

// Review records a 1-5 rating for a finished session and refreshes the tutor's
// aggregate rating. Allowed once, by the learner, after the session ended.
func (s *Service) Review(ctx context.Context, learnerID, bookingID string, rating int, comment string) (*domain.Review, error) {
	if rating < 1 || rating > 5 {
		return nil, apperr.New("tutor.bad_rating", apperr.KindValidation)
	}
	b, err := s.repo.Booking(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if b == nil || b.LearnerID != learnerID {
		return nil, apperr.New("tutor.booking_not_found", apperr.KindNotFound)
	}
	if b.EffectiveStatus(s.now()) != domain.StatusCompleted {
		return nil, apperr.New("tutor.session_not_finished", apperr.KindValidation)
	}
	if existing, _ := s.repo.ReviewByBooking(ctx, bookingID); existing != nil {
		return nil, apperr.New("tutor.already_reviewed", apperr.KindConflict)
	}
	rv := &domain.Review{
		ID:        id.New(),
		BookingID: b.ID,
		TutorID:   b.TutorID,
		LearnerID: learnerID,
		Rating:    rating,
		Comment:   strings.TrimSpace(comment),
		CreatedAt: s.now(),
	}
	if err := s.repo.CreateReview(ctx, rv); err != nil {
		return nil, err
	}
	_ = s.repo.RecomputeRating(ctx, b.TutorID)
	s.notifyTutor(ctx, b.TutorID, domain.NotifReviewNew, map[string]string{"rating": strconv.Itoa(rating)})
	return rv, nil
}

// RoomInfo is the descriptor a client needs to join the live session. Token is a
// placeholder; a production SFU (LiveKit/mediasoup) mints a signed join token.
type RoomInfo struct {
	RoomID     string `json:"room_id"`
	Token      string `json:"token"`
	Provider   string `json:"provider"`
	JoinableAt string `json:"joinable_at"`
	Status     string `json:"status"`
	JoinState  string `json:"join_state"` // open | early | finished
	StartUTC   string `json:"start_utc"`
	EndUTC     string `json:"end_utc"`
	IsHost     bool   `json:"is_host"`
}

// Room returns join info for a booking the learner owns. The session opens a few
// minutes before start and closes after it ends.
func (s *Service) Room(ctx context.Context, learnerID, bookingID string) (*RoomInfo, error) {
	b, err := s.repo.Booking(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if b == nil || b.LearnerID != learnerID {
		return nil, apperr.New("tutor.booking_not_found", apperr.KindNotFound)
	}
	if b.Status == domain.StatusCancelled {
		return nil, apperr.New("tutor.booking_cancelled", apperr.KindValidation)
	}
	state, opensAt, end := roomJoinState(s.now(), b.StartUTC, b.DurationMin, s.windowMin(ctx))
	return &RoomInfo{
		RoomID:     b.RoomID,
		Token:      "stub-" + b.RoomID + "-" + learnerID, // SFU integration point
		Provider:   "stub",
		JoinableAt: opensAt.Format(time.RFC3339),
		Status:     b.EffectiveStatus(s.now()),
		JoinState:  state,
		StartUTC:   b.StartUTC.Format(time.RFC3339),
		EndUTC:     end.Format(time.RFC3339),
		IsHost:     false,
	}, nil
}

// CanJoin reports whether a user may join a booking's live signaling room and
// returns the room id. Currently the booking's learner qualifies; tutor user
// accounts will join here too once tutors are linked to users.
func (s *Service) CanJoin(ctx context.Context, userID, bookingID string) (bool, string, error) {
	b, err := s.repo.Booking(ctx, bookingID)
	if err != nil {
		return false, "", err
	}
	if b == nil || b.Status == domain.StatusCancelled {
		return false, "", nil
	}
	authorized := b.LearnerID == userID
	if !authorized {
		// The tutor (via their linked user account) may also join their own room.
		if t, _ := s.repo.Tutor(ctx, b.TutorID); t != nil && t.UserID != "" && t.UserID == userID {
			authorized = true
		}
	}
	if !authorized {
		return false, "", nil
	}
	// Time gate: the room is only joinable within its window.
	if state, _, _ := roomJoinState(s.now(), b.StartUTC, b.DurationMin, s.windowMin(ctx)); state != "open" {
		return false, "", nil
	}
	return true, b.RoomID, nil
}

// IsStudentOf reports whether studentID has a (non-cancelled) booking with the
// tutor linked to tutorUserID. Used to authorize a tutor viewing a student.
func (s *Service) IsStudentOf(ctx context.Context, tutorUserID, studentID string) (bool, error) {
	t, err := s.repo.TutorByUser(ctx, tutorUserID)
	if err != nil || t == nil {
		return false, err
	}
	bs, err := s.repo.BookingsByTutor(ctx, t.ID)
	if err != nil {
		return false, err
	}
	for _, b := range bs {
		if b.LearnerID == studentID && b.Status != domain.StatusCancelled {
			return true, nil
		}
	}
	return false, nil
}

// IsTutor reports whether a user account is linked to a tutor profile.
func (s *Service) IsTutor(ctx context.Context, userID string) (bool, error) {
	t, err := s.repo.TutorByUser(ctx, userID)
	if err != nil {
		return false, err
	}
	return t != nil && t.Status == domain.StatusApproved, nil
}

// CreateTutorInput holds the fields for onboarding a new tutor profile.
type CreateTutorInput struct {
	Name       string
	Headline   string
	Bio        string
	AvatarURL  string
	Languages  []string
	HourlyGems int
}

// CreateTutor onboards a new tutor profile (admin).
func (s *Service) CreateTutor(ctx context.Context, in CreateTutorInput) (*domain.Tutor, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.ErrBadRequest
	}
	langs := in.Languages
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	t := &domain.Tutor{
		ID:         "tutor-" + id.New(),
		Name:       name,
		Headline:   strings.TrimSpace(in.Headline),
		Bio:        strings.TrimSpace(in.Bio),
		Languages:  langs,
		AvatarURL:  strings.TrimSpace(in.AvatarURL),
		HourlyGems: in.HourlyGems,
		Status:     domain.StatusApproved, // admin-created tutors are pre-approved
	}
	if err := s.repo.CreateTutor(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// CreateTutorWithAccountInput creates a tutor AND a login account for them.
type CreateTutorWithAccountInput struct {
	CreateTutorInput
	Email    string
	Password string
}

// CreateTutorWithAccount provisions a new teacher end-to-end: it creates the
// login account, grants the teacher role, and creates an approved tutor profile
// linked to that account. The teacher can sign in immediately with the given
// email + password.
func (s *Service) CreateTutorWithAccount(ctx context.Context, in CreateTutorWithAccountInput) (*domain.Tutor, error) {
	name := strings.TrimSpace(in.Name)
	email := strings.TrimSpace(in.Email)
	if name == "" || email == "" || in.Password == "" {
		return nil, apperr.ErrBadRequest
	}
	if s.users == nil || s.roles == nil {
		return nil, apperr.ErrInternal
	}
	userID, err := s.users.CreateUser(ctx, email, name, in.Password)
	if err != nil {
		return nil, err // surfaces duplicate-email / validation errors
	}
	if err := s.roles.AssignRole(ctx, userID, roleTeacher); err != nil {
		return nil, apperr.ErrInternal
	}
	langs := in.Languages
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	t := &domain.Tutor{
		ID:         "tutor-" + id.New(),
		UserID:     userID,
		Name:       name,
		Headline:   strings.TrimSpace(in.Headline),
		Bio:        strings.TrimSpace(in.Bio),
		Languages:  langs,
		AvatarURL:  strings.TrimSpace(in.AvatarURL),
		HourlyGems: in.HourlyGems,
		Status:     domain.StatusApproved,
	}
	if err := s.repo.CreateTutor(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateTutorInput carries the editable tutor profile fields.
type UpdateTutorInput struct {
	Name       string
	Headline   string
	Bio        string
	AvatarURL  string
	Languages  []string
	HourlyGems int
	Status     string
}

// UpdateTutor edits an existing tutor's profile.
func (s *Service) UpdateTutor(ctx context.Context, tutorID string, in UpdateTutorInput) (*domain.Tutor, error) {
	t, err := s.repo.Tutor(ctx, tutorID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, apperr.New("tutor.not_found", apperr.KindNotFound)
	}
	if name := strings.TrimSpace(in.Name); name != "" {
		t.Name = name
	}
	t.Headline = strings.TrimSpace(in.Headline)
	t.Bio = strings.TrimSpace(in.Bio)
	t.AvatarURL = strings.TrimSpace(in.AvatarURL)
	if len(in.Languages) > 0 {
		t.Languages = in.Languages
	}
	if in.HourlyGems >= 0 {
		t.HourlyGems = in.HourlyGems
	}
	switch in.Status {
	case domain.StatusApproved, domain.StatusPending, domain.StatusRejected:
		t.Status = in.Status
	}
	if err := s.repo.UpdateTutor(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// SetTutorPassword resets the password of the tutor's linked login account.
func (s *Service) SetTutorPassword(ctx context.Context, tutorID, password string) error {
	if s.users == nil {
		return apperr.ErrInternal
	}
	t, err := s.repo.Tutor(ctx, tutorID)
	if err != nil {
		return err
	}
	if t == nil {
		return apperr.New("tutor.not_found", apperr.KindNotFound)
	}
	if t.UserID == "" {
		return apperr.New("tutor.no_account", apperr.KindValidation)
	}
	return s.users.AdminSetPassword(ctx, t.UserID, password)
}

// DeleteTutor removes a tutor profile and revokes the teacher role from its
// linked account (the account itself is kept).
func (s *Service) DeleteTutor(ctx context.Context, tutorID string) error {
	t, err := s.repo.Tutor(ctx, tutorID)
	if err != nil {
		return err
	}
	if t == nil {
		return apperr.New("tutor.not_found", apperr.KindNotFound)
	}
	if t.UserID != "" && s.roles != nil {
		_ = s.roles.RemoveRole(ctx, t.UserID, roleTeacher)
	}
	return s.repo.DeleteTutor(ctx, tutorID)
}

// Apply lets a signed-in user submit a tutor application (status: pending).
func (s *Service) Apply(ctx context.Context, userID string, in CreateTutorInput) (*domain.Tutor, error) {
	if existing, _ := s.repo.TutorByUser(ctx, userID); existing != nil {
		if existing.Status == domain.StatusApproved {
			return nil, apperr.New("tutor.already_tutor", apperr.KindConflict)
		}
		return nil, apperr.New("tutor.already_applied", apperr.KindConflict)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.ErrBadRequest
	}
	langs := in.Languages
	if len(langs) == 0 {
		langs = []string{"en"}
	}
	t := &domain.Tutor{
		ID:         "tutor-" + id.New(),
		UserID:     userID,
		Name:       name,
		Headline:   strings.TrimSpace(in.Headline),
		Bio:        strings.TrimSpace(in.Bio),
		Languages:  langs,
		AvatarURL:  strings.TrimSpace(in.AvatarURL),
		HourlyGems: in.HourlyGems,
		Status:     domain.StatusPending,
	}
	if err := s.repo.CreateTutor(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Approve activates a tutor (status: approved), grants the teacher role to the
// linked account, and notifies them.
func (s *Service) Approve(ctx context.Context, tutorID string) error {
	t, err := s.repo.Tutor(ctx, tutorID)
	if err != nil {
		return err
	}
	if t == nil {
		return apperr.New("tutor.not_found", apperr.KindNotFound)
	}
	if err := s.repo.SetStatus(ctx, tutorID, domain.StatusApproved); err != nil {
		return err
	}
	if t.UserID != "" {
		if s.roles != nil {
			_ = s.roles.AssignRole(ctx, t.UserID, roleTeacher)
		}
		if s.notif != nil {
			_ = s.notif.Notify(ctx, t.UserID, domain.NotifTutorApproved, map[string]string{}, "/teach")
		}
	}
	return nil
}

// Reject declines a tutor application (status: rejected) and notifies them.
func (s *Service) Reject(ctx context.Context, tutorID, reason string) error {
	t, err := s.repo.Tutor(ctx, tutorID)
	if err != nil {
		return err
	}
	if t == nil {
		return apperr.New("tutor.not_found", apperr.KindNotFound)
	}
	if err := s.repo.SetStatus(ctx, tutorID, domain.StatusRejected); err != nil {
		return err
	}
	if t.UserID != "" && s.notif != nil {
		_ = s.notif.Notify(ctx, t.UserID, domain.NotifTutorRejected, map[string]string{"reason": strings.TrimSpace(reason)}, "")
	}
	return nil
}

// LinkUserByEmail links a tutor to a user account found by email (admin).
func (s *Service) LinkUserByEmail(ctx context.Context, tutorID, email string) error {
	if s.users == nil {
		return apperr.New("tutor.user_not_found", apperr.KindNotFound)
	}
	uid, _, ok, err := s.users.UserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.New("tutor.user_not_found", apperr.KindNotFound)
	}
	return s.LinkUser(ctx, tutorID, uid)
}

// AdminTutorView is a tutor profile plus its link status for the admin console.
type AdminTutorView struct {
	domain.Tutor
	Linked         bool   `json:"linked"`
	LinkedUserName string `json:"linked_user_name,omitempty"`
}

// AdminList returns all tutors with their linked-user name (admin).
func (s *Service) AdminList(ctx context.Context) ([]AdminTutorView, error) {
	ts, err := s.repo.ListTutors(ctx, "")
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(ts))
	for _, t := range ts {
		if t.UserID != "" {
			ids = append(ids, t.UserID)
		}
	}
	nm := s.names(ctx, ids)
	out := make([]AdminTutorView, 0, len(ts))
	for _, t := range ts {
		v := AdminTutorView{Tutor: t, Linked: t.UserID != ""}
		if t.UserID != "" {
			v.LinkedUserName = nm[t.UserID]
		}
		out = append(out, v)
	}
	return out, nil
}

// LinkUser binds a tutor profile to a user account (admin onboarding).
func (s *Service) LinkUser(ctx context.Context, tutorID, userID string) error {
	t, err := s.repo.Tutor(ctx, tutorID)
	if err != nil {
		return err
	}
	if t == nil {
		return apperr.New("tutor.not_found", apperr.KindNotFound)
	}
	if err := s.repo.LinkUser(ctx, tutorID, userID); err != nil {
		return err
	}
	// A linked tutor is a teacher: scope their account to the teaching portal.
	if s.roles != nil {
		_ = s.roles.AssignRole(ctx, userID, roleTeacher)
	}
	if s.notif != nil {
		_ = s.notif.Notify(ctx, userID, domain.NotifTutorWelcome, map[string]string{}, "/teach")
	}
	return nil
}

// TutorForUser returns the tutor profile linked to a user, or nil if none.
func (s *Service) TutorForUser(ctx context.Context, userID string) (*domain.Tutor, error) {
	t, err := s.repo.TutorByUser(ctx, userID)
	if err != nil || t == nil {
		return nil, err
	}
	if t.Status != domain.StatusApproved {
		return nil, nil // not an active tutor yet
	}
	return t, nil
}

// MyApplication returns the user's tutor row regardless of status, for showing
// their application state ("pending"/"rejected") in the UI.
func (s *Service) MyApplication(ctx context.Context, userID string) (*domain.Tutor, error) {
	return s.repo.TutorByUser(ctx, userID)
}

// TutorBookingView is a booking enriched with the student's display name.
type TutorBookingView struct {
	domain.Booking
	LearnerName string
}

// BookingsForTutor returns the schedule (with student names) + profile for the
// tutor linked to a user. A nil tutor means the user is not a tutor.
func (s *Service) BookingsForTutor(ctx context.Context, userID string) ([]TutorBookingView, *domain.Tutor, error) {
	t, err := s.repo.TutorByUser(ctx, userID)
	if err != nil || t == nil {
		return nil, t, err
	}
	bs, err := s.repo.BookingsByTutor(ctx, t.ID)
	if err != nil {
		return nil, t, err
	}
	ids := make([]string, 0, len(bs))
	for _, b := range bs {
		ids = append(ids, b.LearnerID)
	}
	nm := s.names(ctx, ids)
	out := make([]TutorBookingView, 0, len(bs))
	for _, b := range bs {
		out = append(out, TutorBookingView{Booking: b, LearnerName: nm[b.LearnerID]})
	}
	return out, t, nil
}

// StudentSummary aggregates a student's relationship with the tutor.
type StudentSummary struct {
	LearnerID string     `json:"learner_id"`
	Name      string     `json:"name"`
	Sessions  int        `json:"sessions"`
	NextUTC   *time.Time `json:"next_utc,omitempty"`
	LastUTC   *time.Time `json:"last_utc,omitempty"`
}

// StudentsForTutor returns the tutor's roster: one row per student with session
// count and next/last session, derived from non-cancelled bookings.
func (s *Service) StudentsForTutor(ctx context.Context, userID string) ([]StudentSummary, *domain.Tutor, error) {
	t, err := s.repo.TutorByUser(ctx, userID)
	if err != nil || t == nil {
		return nil, t, err
	}
	bs, err := s.repo.BookingsByTutor(ctx, t.ID)
	if err != nil {
		return nil, t, err
	}
	now := s.now()
	type agg struct {
		sessions   int
		next, last *time.Time
	}
	byLearner := map[string]*agg{}
	order := []string{}
	for _, b := range bs {
		if b.Status == domain.StatusCancelled {
			continue
		}
		a := byLearner[b.LearnerID]
		if a == nil {
			a = &agg{}
			byLearner[b.LearnerID] = a
			order = append(order, b.LearnerID)
		}
		a.sessions++
		st := b.StartUTC
		if st.After(now) {
			if a.next == nil || st.Before(*a.next) {
				v := st
				a.next = &v
			}
		} else if a.last == nil || st.After(*a.last) {
			v := st
			a.last = &v
		}
	}
	nm := s.names(ctx, order)
	out := make([]StudentSummary, 0, len(order))
	for _, id := range order {
		a := byLearner[id]
		out = append(out, StudentSummary{LearnerID: id, Name: nm[id], Sessions: a.sessions, NextUTC: a.next, LastUTC: a.last})
	}
	return out, t, nil
}

// SessionLine is one row in a tutor's recent-session list.
type SessionLine struct {
	StartUTC    string `json:"start_utc"`
	DurationMin int    `json:"duration_min"`
	StudentName string `json:"student_name"` // student name (1:1) or class title (group)
	Status      string `json:"status"`       // completed | confirmed
	Kind        string `json:"kind"`         // private | group
	Attendees   int    `json:"attendees"`    // 1 for private, enrolled count for group
}

// EarningsReport summarizes a tutor's teaching activity and payout. Earnings are
// derived from completed teaching time at the admin-configured hourly rate.
type EarningsReport struct {
	PayoutPerHourToman int           `json:"payout_per_hour_toman"`
	TaughtMinutes      int           `json:"taught_minutes"`
	CompletedSessions  int           `json:"completed_sessions"`
	UpcomingSessions   int           `json:"upcoming_sessions"`
	CancelledSessions  int           `json:"cancelled_sessions"`
	UniqueStudents     int           `json:"unique_students"`
	GroupCompleted     int           `json:"group_completed"`
	GroupUpcoming      int           `json:"group_upcoming"`
	GroupAttendees     int           `json:"group_attendees"`
	EarningsToman      int           `json:"earnings_toman"`
	Recent             []SessionLine `json:"recent"`
}

// EarningsReport builds the teaching/earnings summary for the tutor linked to
// userID. Returns tutor.not_a_tutor if the user has no (approved) tutor profile.
func (s *Service) EarningsReport(ctx context.Context, userID string) (*EarningsReport, error) {
	t, err := s.TutorForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
	}
	bookings, err := s.repo.BookingsByTutor(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	rate := s.payoutPerHour(ctx)

	rep := &EarningsReport{PayoutPerHourToman: rate}
	students := map[string]bool{}

	// Unified recent log across 1:1 bookings and group classes.
	type entry struct {
		start time.Time
		line  SessionLine
	}
	var entries []entry

	// Resolve 1:1 student names in one lookup.
	ids := make([]string, 0, len(bookings))
	for _, b := range bookings {
		ids = append(ids, b.LearnerID)
	}
	names := s.names(ctx, ids)

	for _, b := range bookings {
		var status string
		switch b.EffectiveStatus(now) {
		case domain.StatusCompleted:
			rep.CompletedSessions++
			rep.TaughtMinutes += b.DurationMin
			status = domain.StatusCompleted
		case domain.StatusConfirmed:
			rep.UpcomingSessions++
			status = domain.StatusConfirmed
		case domain.StatusCancelled:
			rep.CancelledSessions++
			continue
		}
		students[b.LearnerID] = true
		entries = append(entries, entry{b.StartUTC, SessionLine{
			StartUTC: b.StartUTC.Format(time.RFC3339), DurationMin: b.DurationMin,
			StudentName: names[b.LearnerID], Status: status, Kind: "private", Attendees: 1,
		}})
	}
	rep.UniqueStudents = len(students)

	// Fold in group classes the tutor scheduled (cross-module, optional).
	if s.groupSess != nil {
		if gs, err := s.groupSess.TutorGroupSessions(ctx, userID); err == nil {
			for _, g := range gs {
				status := domain.StatusConfirmed
				if g.Completed {
					status = domain.StatusCompleted
					rep.CompletedSessions++
					rep.GroupCompleted++
					rep.TaughtMinutes += g.DurationMin
				} else {
					rep.UpcomingSessions++
					rep.GroupUpcoming++
				}
				rep.GroupAttendees += g.Attendees
				entries = append(entries, entry{g.StartUTC, SessionLine{
					StartUTC: g.StartUTC.Format(time.RFC3339), DurationMin: g.DurationMin,
					StudentName: g.Title, Status: status, Kind: "group", Attendees: g.Attendees,
				}})
			}
		}
	}

	rep.EarningsToman = rep.TaughtMinutes * rate / 60

	// Most recent first; cap at 8 rows.
	sort.Slice(entries, func(i, j int) bool { return entries[i].start.After(entries[j].start) })
	if len(entries) > 8 {
		entries = entries[:8]
	}
	rep.Recent = make([]SessionLine, 0, len(entries))
	for _, e := range entries {
		rep.Recent = append(rep.Recent, e.line)
	}
	return rep, nil
}

// PayoutSummary is the platform-wide tutor payout liability, for the admin.
type PayoutSummary struct {
	PayoutPerHourToman int `json:"payout_per_hour_toman"`
	TaughtMinutes      int `json:"taught_minutes"`
	PrivateMinutes     int `json:"private_minutes"`
	GroupMinutes       int `json:"group_minutes"`
	PayoutToman        int `json:"payout_toman"`
}

// TotalPayout aggregates completed teaching time across all tutors (1:1 + group)
// and values it at the configured hourly rate.
func (s *Service) TotalPayout(ctx context.Context) (*PayoutSummary, error) {
	now := s.now()
	rate := s.payoutPerHour(ctx)
	sum := &PayoutSummary{PayoutPerHourToman: rate}

	bookings, err := s.repo.AllBookings(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range bookings {
		if b.EffectiveStatus(now) == domain.StatusCompleted {
			sum.PrivateMinutes += b.DurationMin
		}
	}
	if s.groupSess != nil {
		if gs, err := s.groupSess.AllGroupSessions(ctx); err == nil {
			for _, g := range gs {
				if g.Completed {
					sum.GroupMinutes += g.DurationMin
				}
			}
		}
	}
	sum.TaughtMinutes = sum.PrivateMinutes + sum.GroupMinutes
	sum.PayoutToman = sum.TaughtMinutes * rate / 60
	return sum, nil
}

// LearnerSession is one session in a learner's history (admin per-user view).
type LearnerSession struct {
	StartUTC    string `json:"start_utc"`
	DurationMin int    `json:"duration_min"`
	Title       string `json:"title"` // tutor name (private) or class title (group)
	Status      string `json:"status"`
	Kind        string `json:"kind"` // private | group
}

// LearnerSessions returns a user's 1:1 bookings and group enrollments, newest
// first (admin per-user timeline).
func (s *Service) LearnerSessions(ctx context.Context, userID string) ([]LearnerSession, error) {
	bookings, err := s.repo.BookingsByLearner(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	type entry struct {
		start time.Time
		line  LearnerSession
	}
	var entries []entry
	tutorNames := map[string]string{}
	for _, b := range bookings {
		name, ok := tutorNames[b.TutorID]
		if !ok {
			if t, _ := s.repo.Tutor(ctx, b.TutorID); t != nil {
				name = t.Name
			}
			tutorNames[b.TutorID] = name
		}
		entries = append(entries, entry{b.StartUTC, LearnerSession{
			StartUTC: b.StartUTC.Format(time.RFC3339), DurationMin: b.DurationMin,
			Title: name, Status: b.EffectiveStatus(now), Kind: "private",
		}})
	}
	if s.groupSess != nil {
		if gs, err := s.groupSess.StudentGroupSessions(ctx, userID); err == nil {
			for _, g := range gs {
				status := domain.StatusConfirmed
				if g.Completed {
					status = domain.StatusCompleted
				}
				entries = append(entries, entry{g.StartUTC, LearnerSession{
					StartUTC: g.StartUTC.Format(time.RFC3339), DurationMin: g.DurationMin,
					Title: g.Title, Status: status, Kind: "group",
				}})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].start.After(entries[j].start) })
	out := make([]LearnerSession, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.line)
	}
	return out, nil
}

// --- teacher self-service availability -------------------------------------

// slotDurations are the accepted lesson lengths (minutes).
var slotDurations = map[int]bool{25: true, 30: true, 50: true, 60: true}

const maxSlotsPerAdd = 50

// MySlots lists the caller's upcoming openings (booked ones included, so the
// teacher sees their real calendar).
func (s *Service) MySlots(ctx context.Context, userID string) ([]domain.Slot, error) {
	t, err := s.TutorForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
	}
	return s.repo.Availability(ctx, t.ID, s.clock().Add(-time.Minute))
}

// AddSlots opens new bookable times for the calling teacher. Past times,
// unknown durations and overlaps with existing openings are rejected;
// duplicates are skipped silently so a double-submit is harmless.
func (s *Service) AddSlots(ctx context.Context, userID string, starts []time.Time, durationMin int) ([]domain.Slot, error) {
	if len(starts) == 0 || len(starts) > maxSlotsPerAdd || !slotDurations[durationMin] {
		return nil, apperr.ErrBadRequest
	}
	t, err := s.TutorForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
	}
	existing, err := s.repo.Availability(ctx, t.ID, s.clock().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	overlaps := func(a time.Time, aMin int) bool {
		aEnd := a.Add(time.Duration(aMin) * time.Minute)
		for _, e := range existing {
			eEnd := e.StartUTC.Add(time.Duration(e.DurationMin) * time.Minute)
			if a.Before(eEnd) && e.StartUTC.Before(aEnd) {
				return true
			}
		}
		return false
	}
	now := s.clock()
	created := []domain.Slot{}
	for _, st := range starts {
		st = st.UTC().Truncate(time.Minute)
		if !st.After(now) {
			return nil, apperr.ErrBadRequest // past/immediate times are a client bug
		}
		if overlaps(st, durationMin) {
			continue // silently skip collisions (idempotent re-submit)
		}
		sl := domain.Slot{ID: id.New(), TutorID: t.ID, StartUTC: st, DurationMin: durationMin}
		if err := s.repo.AddSlot(ctx, &sl); err != nil {
			return nil, err
		}
		existing = append(existing, sl)
		created = append(created, sl)
	}
	return created, nil
}

// RemoveSlot deletes one of the caller's own, still-unbooked openings.
func (s *Service) RemoveSlot(ctx context.Context, userID, slotID string) error {
	t, err := s.TutorForUser(ctx, userID)
	if err != nil {
		return err
	}
	if t == nil {
		return apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
	}
	sl, err := s.repo.Slot(ctx, slotID)
	if err != nil {
		return err
	}
	if sl == nil || sl.TutorID != t.ID {
		return apperr.New("tutor.slot_not_found", apperr.KindNotFound)
	}
	if sl.Booked {
		return apperr.New("tutor.slot_booked", apperr.KindConflict)
	}
	return s.repo.DeleteSlot(ctx, slotID)
}

// TutorCancel lets the teacher cancel one of their own upcoming classes
// (illness etc.). Unlike a learner cancellation — which frees the slot so
// someone else can book it — the slot is DELETED here: the teacher just told
// us they are not available at that time. The learner is notified.
func (s *Service) TutorCancel(ctx context.Context, userID, bookingID string) error {
	t, err := s.TutorForUser(ctx, userID)
	if err != nil {
		return err
	}
	if t == nil {
		return apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
	}
	b, err := s.repo.Booking(ctx, bookingID)
	if err != nil {
		return err
	}
	if b == nil || b.TutorID != t.ID {
		return apperr.New("tutor.booking_not_found", apperr.KindNotFound)
	}
	if b.Status == domain.StatusCancelled {
		return nil // idempotent
	}
	if !b.StartUTC.After(s.clock()) {
		return apperr.New("tutor.too_late_to_cancel", apperr.KindValidation)
	}
	if err := s.repo.UpdateBookingStatus(ctx, b.ID, domain.StatusCancelled); err != nil {
		return err
	}
	if s.notif != nil {
		_ = s.notif.Notify(ctx, b.LearnerID, domain.NotifTutorCancelled, map[string]string{
			"tutor_name": t.Name, "when": b.StartUTC.Format(time.RFC3339),
		}, "/classes")
	}
	return s.repo.DeleteSlot(ctx, b.SlotID)
}

// UpdateMyProfileInput is the teacher's self-service profile patch. Empty
// strings keep the prior value (same convention as the auth profile);
// languages replace only when provided; a zero rate keeps the prior rate.
type UpdateMyProfileInput struct {
	Headline   string
	Bio        string
	AvatarURL  string
	Languages  []string
	HourlyGems int
}

const (
	maxHeadlineLen = 120
	maxBioLen      = 2000
	maxLanguages   = 6
	maxHourlyGems  = 1000
)

// UpdateMyProfile lets an approved teacher edit their own public listing.
// The payout rate is platform-wide (see payoutPerHour), so the hourly figure
// here is marketplace display only — bounded to keep listings sane.
func (s *Service) UpdateMyProfile(ctx context.Context, userID string, in UpdateMyProfileInput) (*domain.Tutor, error) {
	t, err := s.TutorForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
	}
	if len(in.Headline) > maxHeadlineLen || len(in.Bio) > maxBioLen ||
		len(in.Languages) > maxLanguages || in.HourlyGems < 0 || in.HourlyGems > maxHourlyGems {
		return nil, apperr.ErrBadRequest
	}
	if v := strings.TrimSpace(in.Headline); v != "" {
		t.Headline = v
	}
	if v := strings.TrimSpace(in.Bio); v != "" {
		t.Bio = v
	}
	if v := strings.TrimSpace(in.AvatarURL); v != "" {
		t.AvatarURL = v
	}
	if len(in.Languages) > 0 {
		langs := make([]string, 0, len(in.Languages))
		for _, l := range in.Languages {
			l = strings.ToLower(strings.TrimSpace(l))
			if l != "" && len(l) <= 8 {
				langs = append(langs, l)
			}
		}
		if len(langs) > 0 {
			t.Languages = langs
		}
	}
	if in.HourlyGems > 0 {
		t.HourlyGems = in.HourlyGems
	}
	if err := s.repo.UpdateTutor(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}
