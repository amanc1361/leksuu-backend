// Package usecase holds group-class logic: a tutor schedules a class, students
// enroll up to capacity, and both join one shared room. CanJoin satisfies the
// rtc Authorizer port so group rooms authorize over the same signaling channel.
package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/classroom/domain"
	"github.com/leksuu/platform/internal/modules/reminder"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

const defaultCapacity = 6

type TutorCheck interface {
	IsTutor(ctx context.Context, userID string) (bool, error)
}
type UserDirectory interface {
	DisplayNames(ctx context.Context, ids []string) (map[string]string, error)
}

// Notifier emits in-app notifications (optional).
type Notifier interface {
	Notify(ctx context.Context, userID, ntype string, data map[string]string, link string) error
}

type Service struct {
	repo        domain.Repository
	tutors      TutorCheck
	users       UserDirectory
	notif       Notifier
	classAccess ClassAccess
	joinWindow  JoinWindow
	sessions    SessionSource
	clock       func() time.Time
	// Paid seats (market.go).
	market   domain.MarketStore
	credits  Credits
	payments ClassPayments
	share    SharePercent
}

// ClassAccess gates group-class enrollment behind a class subscription (optional).
type ClassAccess interface {
	RequireClass(ctx context.Context, userID string) error
}

// JoinWindow supplies the live-room join window in minutes (optional).
type JoinWindow interface {
	RoomJoinWindowMin(ctx context.Context) int
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

func roomOpen(now, start time.Time, durationMin, windowMin int) bool {
	state, _, _ := roomJoinState(now, start, durationMin, windowMin)
	return state == "open"
}

// RoomInfo is the join payload for a group-class room (mirrors the tutor shape).
type RoomInfo struct {
	RoomID     string `json:"room_id"`
	Token      string `json:"token"`
	Provider   string `json:"provider"`
	JoinableAt string `json:"joinable_at"`
	Status     string `json:"status"`
	JoinState  string `json:"join_state"` // open | early | finished
	StartUTC   string `json:"start_utc"`
	EndUTC     string `json:"end_utc"`
	IsHost     bool   `json:"is_host"` // the class tutor manages the room
}

// Room returns join info for a group class the user may attend (tutor or
// enrolled student), including the time-window state for a join countdown.
func (s *Service) Room(ctx context.Context, userID, classID string) (*RoomInfo, error) {
	c, err := s.repo.GetClass(ctx, classID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, apperr.New("classroom.not_found", apperr.KindNotFound)
	}
	authorized := c.TutorID == userID
	if !authorized {
		enrolled, err := s.repo.IsEnrolled(ctx, classID, userID)
		if err != nil {
			return nil, err
		}
		authorized = enrolled
	}
	if !authorized {
		return nil, apperr.New("classroom.not_enrolled", apperr.KindForbidden)
	}
	state, opensAt, end := roomJoinState(s.now(), c.StartUTC, c.DurationMin, s.windowMin(ctx))
	status := "upcoming"
	if state == "open" {
		status = "live"
	} else if state == "finished" {
		status = "ended"
	}
	return &RoomInfo{
		RoomID:     c.RoomID,
		Token:      "stub-" + c.RoomID + "-" + userID,
		Provider:   "stub",
		JoinableAt: opensAt.Format(time.RFC3339),
		Status:     status,
		JoinState:  state,
		StartUTC:   c.StartUTC.Format(time.RFC3339),
		EndUTC:     end.Format(time.RFC3339),
		IsHost:     c.TutorID == userID,
	}, nil
}

// SetNotifier wires the notification emitter (optional; nil-safe).
func (s *Service) SetNotifier(n Notifier) { s.notif = n }

// Access reports whether a user is the class host (tutor) and whether they may
// view its contents (host or enrolled student). Used to authorize recordings.
func (s *Service) Access(ctx context.Context, classID, userID string) (isHost bool, canView bool, err error) {
	c, err := s.repo.GetClass(ctx, classID)
	if err != nil {
		return false, false, err
	}
	if c == nil {
		return false, false, apperr.New("classroom.not_found", apperr.KindNotFound)
	}
	if c.TutorID == userID {
		return true, true, nil
	}
	enrolled, err := s.repo.IsEnrolled(ctx, classID, userID)
	if err != nil {
		return false, false, err
	}
	return false, enrolled, nil
}

// SetClassAccess wires the class-subscription gate for enrollment (optional).
func (s *Service) SetClassAccess(c ClassAccess) { s.classAccess = c }

// SetJoinWindow wires the room join-window source (optional; default 10 min).
func (s *Service) SetJoinWindow(j JoinWindow) { s.joinWindow = j }

// SessionSource supplies 1:1 session start times so platform class stats reflect
// every kind of live class, not just group classes (optional; nil = group-only).
type SessionSource interface {
	SessionTimes(ctx context.Context) ([]time.Time, error)
}

func (s *Service) SetSessionSource(src SessionSource) { s.sessions = src }

func (s *Service) windowMin(ctx context.Context) int {
	if s.joinWindow != nil {
		if m := s.joinWindow.RoomJoinWindowMin(ctx); m > 0 {
			return m
		}
	}
	return 10
}

func NewService(repo domain.Repository, tutors TutorCheck, users UserDirectory, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, tutors: tutors, users: users, clock: clock}
}

func (s *Service) now() time.Time { return s.clock().UTC() }

func (s *Service) names(ctx context.Context, ids ...string) map[string]string {
	if s.users == nil {
		return map[string]string{}
	}
	m, _ := s.users.DisplayNames(ctx, ids)
	if m == nil {
		return map[string]string{}
	}
	return m
}

// ClassView is a class plus derived fields for browsing/scheduling.
type ClassView struct {
	domain.ClassSession
	TutorName string `json:"tutor_name,omitempty"`
	Enrolled  int    `json:"enrolled"`
	IsFull    bool   `json:"is_full"`
	Joined    bool   `json:"joined"`
	// PriceToman is the price per seat (0 = only old class subscriptions).
	PriceToman int `json:"price_toman"`
	// Paid seats not refunded (incl. late leavers) and the teacher's part.
	RevenueToman       int `json:"revenue_toman,omitempty"`
	TeacherEarnedToman int `json:"teacher_earned_toman,omitempty"`
}

type CreateInput struct {
	Title       string
	Description string
	StartUTC    time.Time
	DurationMin int
	Capacity    int
	PriceToman  int // price per seat (required when seats are sold)
}

// Create schedules a new group class (tutors only).
func (s *Service) Create(ctx context.Context, tutorUserID string, in CreateInput) (*domain.ClassSession, error) {
	if s.tutors != nil {
		ok, err := s.tutors.IsTutor(ctx, tutorUserID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
		}
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return nil, apperr.ErrBadRequest
	}
	if in.StartUTC.Before(s.now()) {
		return nil, apperr.New("classroom.start_past", apperr.KindValidation)
	}
	if in.DurationMin <= 0 {
		in.DurationMin = 30
	}
	if s.market != nil && in.PriceToman <= 0 {
		return nil, errClassPrice
	}
	if in.Capacity <= 0 {
		in.Capacity = defaultCapacity
	}
	c := &domain.ClassSession{
		ID:          id.New(),
		TutorID:     tutorUserID,
		Title:       in.Title,
		Description: strings.TrimSpace(in.Description),
		StartUTC:    in.StartUTC.UTC(),
		DurationMin: in.DurationMin,
		Capacity:    in.Capacity,
		CreatedAt:   s.now(),
	}
	c.RoomID = "class_" + c.ID
	if err := s.repo.CreateClass(ctx, c); err != nil {
		return nil, err
	}
	if s.market != nil && in.PriceToman > 0 {
		if err := s.market.SetClassPrice(ctx, c.ID, in.PriceToman); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (s *Service) view(ctx context.Context, c domain.ClassSession, viewerID string, tutorNames map[string]string) ClassView {
	enrolled, _ := s.repo.EnrollmentCount(ctx, c.ID)
	joined := false
	if viewerID != "" {
		joined, _ = s.repo.IsEnrolled(ctx, c.ID, viewerID)
	}
	name := ""
	if tutorNames != nil {
		name = tutorNames[c.TutorID]
	}
	return ClassView{ClassSession: c, TutorName: name, Enrolled: enrolled, IsFull: enrolled >= c.Capacity, Joined: joined}
}

// Browse lists upcoming group classes a student can enroll in.
func (s *Service) Browse(ctx context.Context, viewerID string) ([]ClassView, error) {
	cs, err := s.repo.ListUpcoming(ctx, s.now())
	if err != nil {
		return nil, err
	}
	return s.toViews(ctx, cs, viewerID), nil
}

// TutorClasses lists the classes a tutor scheduled (with enrollment counts).
func (s *Service) TutorClasses(ctx context.Context, tutorUserID string) ([]ClassView, error) {
	cs, err := s.repo.ByTutor(ctx, tutorUserID)
	if err != nil {
		return nil, err
	}
	return s.toViews(ctx, cs, tutorUserID), nil
}

// ClassStats summarizes group-class counts for the admin overview.
type ClassStats struct {
	Total    int `json:"total"`
	Today    int `json:"today"`
	Upcoming int `json:"upcoming"`
}

// ClassStats counts all/today/upcoming group classes (admin overview).
func (s *Service) ClassStats(ctx context.Context) (*ClassStats, error) {
	cs, err := s.repo.AllClasses(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	y, m, d := now.UTC().Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	out := &ClassStats{Total: len(cs)}
	for _, c := range cs {
		if c.StartUTC.After(now) {
			out.Upcoming++
		}
		if !c.StartUTC.Before(start) && c.StartUTC.Before(end) {
			out.Today++
		}
	}
	// Include 1:1 live sessions so "classes" means the same thing platform-wide.
	if s.sessions != nil {
		ts, err := s.sessions.SessionTimes(ctx)
		if err != nil {
			return nil, err
		}
		out.Total += len(ts)
		for _, t := range ts {
			if t.After(now) {
				out.Upcoming++
			}
			if !t.Before(start) && t.Before(end) {
				out.Today++
			}
		}
	}
	return out, nil
}

// AllClasses lists every group class (admin/analytics use), with enrollment counts.
func (s *Service) AllClasses(ctx context.Context) ([]ClassView, error) {
	cs, err := s.repo.AllClasses(ctx)
	if err != nil {
		return nil, err
	}
	return s.toViews(ctx, cs, ""), nil
}

// MyClasses lists the classes a student is enrolled in.
func (s *Service) MyClasses(ctx context.Context, studentID string) ([]ClassView, error) {
	cs, err := s.repo.ClassesForStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}
	return s.toViews(ctx, cs, studentID), nil
}

func (s *Service) toViews(ctx context.Context, cs []domain.ClassSession, viewerID string) []ClassView {
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.TutorID)
	}
	names := s.names(ctx, ids...)
	out := make([]ClassView, 0, len(cs))
	for _, c := range cs {
		out = append(out, s.view(ctx, c, viewerID, names))
	}
	return s.withMoney(ctx, out)
}

// enrollLegacy enrolls under the old class subscription (no charge per seat).
func (s *Service) enrollLegacy(ctx context.Context, studentID, classID string) error {
	c, err := s.repo.GetClass(ctx, classID)
	if err != nil {
		return err
	}
	if c == nil {
		return apperr.New("classroom.not_found", apperr.KindNotFound)
	}
	if c.EndUTC().Before(s.now()) {
		return apperr.New("classroom.finished", apperr.KindValidation)
	}
	if already, _ := s.repo.IsEnrolled(ctx, classID, studentID); already {
		return apperr.New("classroom.already_enrolled", apperr.KindConflict)
	}
	if n, _ := s.repo.EnrollmentCount(ctx, classID); n >= c.Capacity {
		return apperr.New("classroom.full", apperr.KindConflict)
	}
	if s.classAccess != nil {
		if err := s.classAccess.RequireClass(ctx, studentID); err != nil {
			return err
		}
	}
	if err := s.repo.Enroll(ctx, classID, studentID); err != nil {
		return err
	}
	if s.notif != nil {
		sn := s.names(ctx, studentID)[studentID]
		_ = s.notif.Notify(ctx, c.TutorID, "class_enrolled", map[string]string{"student_name": sn, "class_title": c.Title}, "/teach")
	}
	return nil
}

// Leave un-enrolls a student. A paid seat left more than 24h before the class
// is refunded as class credit; later, the teacher keeps their share.
func (s *Service) Leave(ctx context.Context, studentID, classID string) error {
	if err := s.refundSeat(ctx, studentID, classID); err != nil {
		return err
	}
	return s.repo.Unenroll(ctx, classID, studentID)
}

// CanJoin authorizes the rtc signaling channel: the tutor or any enrolled
// student may join the class room. Satisfies the rtc Authorizer port.
func (s *Service) CanJoin(ctx context.Context, userID, classID string) (bool, string, error) {
	c, err := s.repo.GetClass(ctx, classID)
	if err != nil || c == nil {
		return false, "", err
	}
	authorized := c.TutorID == userID
	if !authorized {
		enrolled, err := s.repo.IsEnrolled(ctx, classID, userID)
		if err != nil {
			return false, "", err
		}
		authorized = enrolled
	}
	if !authorized {
		return false, "", nil
	}
	if !roomOpen(s.now(), c.StartUTC, c.DurationMin, s.windowMin(ctx)) {
		return false, "", nil
	}
	return true, c.RoomID, nil
}

// DueReminders builds class-reminder notifications for group classes starting
// within [from, to]. Satisfies reminder.Source. The tutor and every enrolled
// student is reminded, all seeing the class title.
func (s *Service) DueReminders(ctx context.Context, from, to time.Time) ([]reminder.Reminder, error) {
	cs, err := s.repo.ClassesStarting(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]reminder.Reminder, 0, len(cs))
	for _, c := range cs {
		roster, _ := s.repo.Roster(ctx, c.ID)
		when := c.StartUTC.Format(time.RFC3339)
		data := map[string]string{"name": c.Title, "when": when}
		targets := make([]reminder.Target, 0, len(roster)+1)
		targets = append(targets, reminder.Target{UserID: c.TutorID, Data: data})
		for _, sid := range roster {
			targets = append(targets, reminder.Target{UserID: sid, Data: data})
		}
		out = append(out, reminder.Reminder{Key: c.ID, Type: reminder.TypeClassReminder, Link: "/room/" + c.ID, Targets: targets})
	}
	return out, nil
}
