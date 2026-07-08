package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/tutor/domain"
	"github.com/leksuu/platform/internal/modules/tutor/repo"
	"github.com/leksuu/platform/internal/shared/apperr"
)

func TestBookCancelReviewFlow(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, clock)
	ctx := context.Background()

	det, err := svc.GetTutor(ctx, "tutor-sara")
	if err != nil {
		t.Fatal(err)
	}
	if len(det.Availability) == 0 {
		t.Fatal("expected available slots")
	}
	slot := det.Availability[0]

	b, err := svc.Book(ctx, "learner-1", slot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.RoomID == "" || b.Status != domain.StatusConfirmed {
		t.Fatalf("bad booking: %+v", b)
	}

	// Double-book the same slot must fail.
	if _, err := svc.Book(ctx, "learner-2", slot.ID); err == nil {
		t.Fatal("expected slot_taken on double book")
	}

	// Cannot review before the session ends.
	if _, err := svc.Review(ctx, "learner-1", b.ID, 5, "great"); err == nil {
		t.Fatal("expected session_not_finished")
	}

	// Advance the clock past the session end, then review.
	now = slot.StartUTC.Add(time.Hour)
	rv, err := svc.Review(ctx, "learner-1", b.ID, 5, "great session")
	if err != nil {
		t.Fatal(err)
	}
	if rv.Rating != 5 {
		t.Fatalf("bad review: %+v", rv)
	}

	// Rating recomputed from reviews only (count becomes 1).
	tt, _ := r.Tutor(ctx, "tutor-sara")
	if tt.RatingCount != 1 || tt.RatingAvg != 5 {
		t.Fatalf("rating not recomputed: avg=%v count=%d", tt.RatingAvg, tt.RatingCount)
	}

	// Reviewing twice must fail.
	if _, err := svc.Review(ctx, "learner-1", b.ID, 4, "again"); err == nil {
		t.Fatal("expected already_reviewed")
	}
}

func TestCancelFreesSlot(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, clock)
	ctx := context.Background()

	det, _ := svc.GetTutor(ctx, "tutor-dilan")
	slot := det.Availability[0]
	b, err := svc.Book(ctx, "learner-1", slot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Cancel(ctx, "learner-1", b.ID); err != nil {
		t.Fatal(err)
	}
	// Slot is bookable again.
	if _, err := svc.Book(ctx, "learner-2", slot.ID); err != nil {
		t.Fatalf("slot should be free after cancel: %v", err)
	}
}

func TestTutorLinkAndRoomAccess(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	ctx := context.Background()

	// Link a user account to the tutor profile.
	if err := svc.LinkUser(ctx, "tutor-sara", "tutor-user-1"); err != nil {
		t.Fatal(err)
	}
	prof, _ := svc.TutorForUser(ctx, "tutor-user-1")
	if prof == nil || prof.ID != "tutor-sara" {
		t.Fatalf("TutorForUser = %+v", prof)
	}

	// A learner books that tutor.
	det, _ := svc.GetTutor(ctx, "tutor-sara")
	b, err := svc.Book(ctx, "learner-1", det.Availability[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	// The room is joinable within its time window.
	now = b.StartUTC
	// Both the learner and the tutor's user may join the room.
	if ok, room, _ := svc.CanJoin(ctx, "learner-1", b.ID); !ok || room != b.RoomID {
		t.Fatal("learner should be able to join")
	}
	if ok, room, _ := svc.CanJoin(ctx, "tutor-user-1", b.ID); !ok || room != b.RoomID {
		t.Fatal("tutor user should be able to join")
	}
	// A stranger may not.
	if ok, _, _ := svc.CanJoin(ctx, "stranger", b.ID); ok {
		t.Fatal("stranger must not join")
	}

	// The tutor sees the booking in their schedule.
	bs, tt, err := svc.BookingsForTutor(ctx, "tutor-user-1")
	if err != nil || tt == nil || len(bs) != 1 || bs[0].ID != b.ID {
		t.Fatalf("tutor bookings = %+v (tutor %v)", bs, tt)
	}

	// Linking an unknown tutor fails.
	if err := svc.LinkUser(ctx, "tutor-nope", "x"); err == nil {
		t.Fatal("expected not_found linking unknown tutor")
	}
}

// fakeDirectory resolves a fixed set of user IDs to names.
type fakeDirectory map[string]string

func (f fakeDirectory) UserByEmail(_ context.Context, email string) (string, string, bool, error) {
	for id, name := range f {
		_ = name
		if id == email {
			return id, f[id], true, nil
		}
	}
	return "", "", false, nil
}

func (f fakeDirectory) DisplayNames(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := f[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

func (f fakeDirectory) CreateUser(_ context.Context, email, displayName, _ string) (string, error) {
	uid := "u-" + email
	f[uid] = displayName
	return uid, nil
}

func (f fakeDirectory) AdminSetPassword(_ context.Context, _, _ string) error { return nil }

func TestTutorStudentsRosterWithNames(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	svc.SetDirectory(fakeDirectory{"u-ali": "Ali", "u-mina": "Mina"})
	ctx := context.Background()
	_ = svc.LinkUser(ctx, "tutor-sara", "tutor-user")

	det, _ := svc.GetTutor(ctx, "tutor-sara")
	// Ali books two slots, Mina one.
	if _, err := svc.Book(ctx, "u-ali", det.Availability[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Book(ctx, "u-ali", det.Availability[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Book(ctx, "u-mina", det.Availability[2].ID); err != nil {
		t.Fatal(err)
	}

	// Bookings carry the resolved student names.
	bs, _, err := svc.BookingsForTutor(ctx, "tutor-user")
	if err != nil || len(bs) != 3 {
		t.Fatalf("bookings = %d", len(bs))
	}
	names := map[string]bool{}
	for _, b := range bs {
		names[b.LearnerName] = true
	}
	if !names["Ali"] || !names["Mina"] {
		t.Fatalf("expected resolved names, got %+v", names)
	}

	// Roster: 2 students, Ali with 2 sessions, each with a future "next".
	roster, _, err := svc.StudentsForTutor(ctx, "tutor-user")
	if err != nil || len(roster) != 2 {
		t.Fatalf("roster = %d", len(roster))
	}
	for _, st := range roster {
		if st.Name == "" || st.NextUTC == nil {
			t.Fatalf("bad roster row: %+v", st)
		}
		if st.LearnerID == "u-ali" && st.Sessions != 2 {
			t.Fatalf("Ali sessions = %d", st.Sessions)
		}
	}
}

type fakeRoles struct{ assigned map[string]string }

func (f *fakeRoles) AssignRole(_ context.Context, userID, role string) error {
	if f.assigned == nil {
		f.assigned = map[string]string{}
	}
	f.assigned[userID] = role
	return nil
}

func (f *fakeRoles) RemoveRole(_ context.Context, userID, role string) error {
	delete(f.assigned, userID)
	return nil
}

func TestTutorApplicationFlow(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	roles := &fakeRoles{}
	svc.SetRoles(roles)
	ctx := context.Background()

	baseList, _ := svc.ListTutors(ctx, "")
	base := len(baseList)

	// Apply -> pending, hidden from public list, not a tutor yet.
	app, err := svc.Apply(ctx, "user-1", CreateTutorInput{Name: "Reza", Languages: []string{"en"}, HourlyGems: 100})
	if err != nil {
		t.Fatal(err)
	}
	if app.Status != domain.StatusPending {
		t.Fatalf("status = %s, want pending", app.Status)
	}
	if l, _ := svc.ListTutors(ctx, ""); len(l) != base {
		t.Fatal("pending tutor must not appear in public list")
	}
	if ok, _ := svc.IsTutor(ctx, "user-1"); ok {
		t.Fatal("pending applicant is not a tutor yet")
	}
	if mt, _ := svc.TutorForUser(ctx, "user-1"); mt != nil {
		t.Fatal("TutorForUser must be nil for a pending applicant")
	}
	// Duplicate apply rejected.
	if _, err := svc.Apply(ctx, "user-1", CreateTutorInput{Name: "x"}); err == nil {
		t.Fatal("duplicate apply must fail")
	}

	// Approve -> appears publicly, becomes a tutor, gets the teacher role.
	if err := svc.Approve(ctx, app.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := svc.ListTutors(ctx, ""); len(l) != base+1 {
		t.Fatal("approved tutor must appear in public list")
	}
	if ok, _ := svc.IsTutor(ctx, "user-1"); !ok {
		t.Fatal("approved applicant must be a tutor")
	}
	if roles.assigned["user-1"] != "teacher" {
		t.Fatalf("teacher role not assigned: %v", roles.assigned)
	}

	// Reject a second applicant -> hidden from public list.
	app2, _ := svc.Apply(ctx, "user-2", CreateTutorInput{Name: "Sina", Languages: []string{"en"}})
	if err := svc.Reject(ctx, app2.ID, "not enough experience"); err != nil {
		t.Fatal(err)
	}
	if l, _ := svc.ListTutors(ctx, ""); len(l) != base+1 {
		t.Fatal("rejected tutor must not appear in public list")
	}
}

type fakeClassAccess struct{ allow bool }

func (f fakeClassAccess) RequireClass(_ context.Context, _ string) error {
	if f.allow {
		return nil
	}
	return apperr.New("billing.class_subscription_required", apperr.KindForbidden)
}

func TestBookingRequiresClassSubscription(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	svc.SetClassAccess(fakeClassAccess{allow: false})
	ctx := context.Background()

	det, _ := svc.GetTutor(ctx, "tutor-sara")
	slot := det.Availability[0]

	// Without a class subscription, booking is refused and the slot stays free.
	if _, err := svc.Book(ctx, "learner-1", slot.ID); err == nil {
		t.Fatal("expected class_subscription_required")
	}
	if sl, _ := r.Slot(ctx, slot.ID); sl.Booked {
		t.Fatal("slot must remain free when access is denied")
	}

	// With an active class subscription, booking succeeds.
	svc.SetClassAccess(fakeClassAccess{allow: true})
	if _, err := svc.Book(ctx, "learner-1", slot.ID); err != nil {
		t.Fatalf("booking with class access failed: %v", err)
	}
}

func TestRoomJoinTimeWindow(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	svc.SetClassAccess(fakeClassAccess{allow: true})
	ctx := context.Background()

	det, _ := svc.GetTutor(ctx, "tutor-sara")
	slot := det.Availability[0] // 30-min slot in the future
	b, err := svc.Book(ctx, "learner-1", slot.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Well before the window: not joinable.
	now = slot.StartUTC.Add(-30 * time.Minute) // default window is 10 min
	if ok, _, _ := svc.CanJoin(ctx, "learner-1", b.ID); ok {
		t.Fatal("must not join 30m before start (window is 10m)")
	}
	// Inside the window (5 min before): joinable.
	now = slot.StartUTC.Add(-5 * time.Minute)
	if ok, _, _ := svc.CanJoin(ctx, "learner-1", b.ID); !ok {
		t.Fatal("must join 5m before start")
	}
	// After the session ends: not joinable.
	now = slot.StartUTC.Add(time.Duration(slot.DurationMin+5) * time.Minute)
	if ok, _, _ := svc.CanJoin(ctx, "learner-1", b.ID); ok {
		t.Fatal("must not join after the session ends")
	}
}

type fakePayout struct{ rate int }

func (f fakePayout) TutorPayoutPerHourToman(_ context.Context) int { return f.rate }

func TestEarningsReport(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	svc.SetClassAccess(fakeClassAccess{allow: true})
	svc.SetPayoutRate(fakePayout{rate: 60000}) // 60,000 toman / hour
	ctx := context.Background()

	if err := svc.LinkUser(ctx, "tutor-sara", "tutor-user-1"); err != nil {
		t.Fatal(err)
	}
	det, _ := svc.GetTutor(ctx, "tutor-sara")
	// Book two slots with two different learners.
	b1, err := svc.Book(ctx, "learner-1", det.Availability[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Book(ctx, "learner-2", det.Availability[1].ID); err != nil {
		t.Fatal(err)
	}

	// Before any session: no completed sessions, no earnings.
	rep, err := svc.EarningsReport(ctx, "tutor-user-1")
	if err != nil {
		t.Fatal(err)
	}
	if rep.CompletedSessions != 0 || rep.UpcomingSessions != 2 || rep.EarningsToman != 0 {
		t.Fatalf("pre-session report wrong: %+v", rep)
	}
	if rep.UniqueStudents != 2 {
		t.Fatalf("want 2 unique students, got %d", rep.UniqueStudents)
	}

	// Advance past the first booking's end: it becomes completed.
	now = b1.StartUTC.Add(time.Duration(b1.DurationMin+1) * time.Minute)
	rep, _ = svc.EarningsReport(ctx, "tutor-user-1")
	if rep.CompletedSessions != 1 || rep.TaughtMinutes != b1.DurationMin {
		t.Fatalf("post-session report wrong: %+v", rep)
	}
	want := b1.DurationMin * 60000 / 60
	if rep.EarningsToman != want {
		t.Fatalf("earnings = %d, want %d", rep.EarningsToman, want)
	}
	if len(rep.Recent) == 0 {
		t.Fatal("recent sessions should not be empty")
	}

	// A non-tutor user is refused.
	if _, err := svc.EarningsReport(ctx, "nobody"); err == nil {
		t.Fatal("non-tutor must be refused")
	}
}

type fakeGroupSessions struct{ sessions []GroupSession }

func (f fakeGroupSessions) TutorGroupSessions(_ context.Context, _ string) ([]GroupSession, error) {
	return f.sessions, nil
}
func (f fakeGroupSessions) AllGroupSessions(_ context.Context) ([]GroupSession, error) {
	return f.sessions, nil
}
func (f fakeGroupSessions) StudentGroupSessions(_ context.Context, _ string) ([]GroupSession, error) {
	return f.sessions, nil
}

func TestEarningsReportIncludesGroupClasses(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	svc.SetClassAccess(fakeClassAccess{allow: true})
	svc.SetPayoutRate(fakePayout{rate: 60000})
	svc.SetGroupSessions(fakeGroupSessions{sessions: []GroupSession{
		{StartUTC: base.Add(-2 * time.Hour), DurationMin: 60, Title: "Speaking circle", Attendees: 4, Completed: true},
		{StartUTC: base.Add(48 * time.Hour), DurationMin: 30, Title: "Grammar clinic", Attendees: 2, Completed: false},
	}})
	ctx := context.Background()
	if err := svc.LinkUser(ctx, "tutor-sara", "tutor-user-1"); err != nil {
		t.Fatal(err)
	}

	rep, err := svc.EarningsReport(ctx, "tutor-user-1")
	if err != nil {
		t.Fatal(err)
	}
	// One completed (60m) + one upcoming group class.
	if rep.GroupCompleted != 1 || rep.GroupUpcoming != 1 {
		t.Fatalf("group counts wrong: %+v", rep)
	}
	if rep.GroupAttendees != 6 {
		t.Fatalf("group attendees = %d, want 6", rep.GroupAttendees)
	}
	// Completed group time folds into taught minutes + earnings.
	if rep.TaughtMinutes != 60 {
		t.Fatalf("taught minutes = %d, want 60", rep.TaughtMinutes)
	}
	if rep.EarningsToman != 60000 { // 60 min * 60000/hr
		t.Fatalf("earnings = %d, want 60000", rep.EarningsToman)
	}
	// Recent log carries both kinds.
	var gotGroup bool
	for _, l := range rep.Recent {
		if l.Kind == "group" && l.StudentName == "Speaking circle" {
			gotGroup = true
		}
	}
	if !gotGroup {
		t.Fatal("recent log must include the group session")
	}
}

func TestTotalPayout(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	svc.SetClassAccess(fakeClassAccess{allow: true})
	svc.SetPayoutRate(fakePayout{rate: 60000})
	svc.SetGroupSessions(fakeGroupSessions{sessions: []GroupSession{
		{StartUTC: base.Add(-3 * time.Hour), DurationMin: 90, Completed: true},  // counts
		{StartUTC: base.Add(24 * time.Hour), DurationMin: 30, Completed: false}, // future, ignored
	}})
	ctx := context.Background()

	// One completed 1:1 booking (30m) across the platform.
	det, _ := svc.GetTutor(ctx, "tutor-sara")
	b, _ := svc.Book(ctx, "learner-1", det.Availability[0].ID)
	now = b.StartUTC.Add(time.Duration(b.DurationMin+1) * time.Minute)

	sum, err := svc.TotalPayout(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.PrivateMinutes != b.DurationMin || sum.GroupMinutes != 90 {
		t.Fatalf("minutes wrong: %+v", sum)
	}
	wantMin := b.DurationMin + 90
	if sum.TaughtMinutes != wantMin {
		t.Fatalf("taught = %d, want %d", sum.TaughtMinutes, wantMin)
	}
	if sum.PayoutToman != wantMin*60000/60 {
		t.Fatalf("payout = %d, want %d", sum.PayoutToman, wantMin*60000/60)
	}
}

func TestLearnerSessions(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	svc.SetClassAccess(fakeClassAccess{allow: true})
	svc.SetGroupSessions(fakeGroupSessions{sessions: []GroupSession{
		{StartUTC: base.Add(-2 * time.Hour), DurationMin: 45, Title: "Speaking circle", Completed: true},
	}})
	ctx := context.Background()

	det, _ := svc.GetTutor(ctx, "tutor-sara")
	if _, err := svc.Book(ctx, "learner-X", det.Availability[0].ID); err != nil {
		t.Fatal(err)
	}

	sessions, err := svc.LearnerSessions(ctx, "learner-X")
	if err != nil {
		t.Fatal(err)
	}
	var priv, grp int
	for _, s := range sessions {
		if s.Kind == "private" {
			priv++
		}
		if s.Kind == "group" && s.Title == "Speaking circle" {
			grp++
		}
	}
	if priv != 1 || grp != 1 {
		t.Fatalf("expected 1 private + 1 group, got priv=%d grp=%d (n=%d)", priv, grp, len(sessions))
	}
	// Newest first: the future 1:1 booking should sort before the past group class.
	if sessions[0].Kind != "private" {
		t.Fatalf("expected newest (private booking) first, got %s", sessions[0].Kind)
	}
}

func TestCreateTutorWithAccountAndDelete(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	roles := &fakeRoles{}
	svc.SetRoles(roles)
	svc.SetDirectory(fakeDirectory{})
	ctx := context.Background()

	tt, err := svc.CreateTutorWithAccount(ctx, CreateTutorWithAccountInput{
		CreateTutorInput: CreateTutorInput{Name: "Sara", Languages: []string{"en"}, HourlyGems: 120},
		Email:            "sara@example.com", Password: "secret123",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tt.UserID == "" || tt.Status != domain.StatusApproved {
		t.Fatalf("tutor not linked/approved: %+v", tt)
	}
	if roles.assigned[tt.UserID] != "teacher" {
		t.Fatalf("teacher role not granted")
	}
	// appears in public list (approved)
	list, _ := svc.ListTutors(ctx, "")
	found := false
	for _, x := range list {
		if x.ID == tt.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("approved tutor should be public")
	}
	// update
	if _, err := svc.UpdateTutor(ctx, tt.ID, UpdateTutorInput{Name: "Sara K", HourlyGems: 200}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := r.Tutor(ctx, tt.ID)
	if got.Name != "Sara K" || got.HourlyGems != 200 {
		t.Fatalf("update not applied: %+v", got)
	}
	// password reset (no error)
	if err := svc.SetTutorPassword(ctx, tt.ID, "newpass"); err != nil {
		t.Fatalf("set password: %v", err)
	}
	// delete -> revokes role + removed
	if err := svc.DeleteTutor(ctx, tt.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := roles.assigned[tt.UserID]; ok {
		t.Fatal("teacher role should be revoked on delete")
	}
	if got, _ := r.Tutor(ctx, tt.ID); got != nil {
		t.Fatal("tutor should be deleted")
	}
}

func TestTeacherAvailabilitySelfService(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	ctx := context.Background()

	// Link a user account to the seeded tutor (same path production uses).
	if err := svc.LinkUser(ctx, "tutor-sara", "user-sara"); err != nil {
		t.Fatal(err)
	}

	before, err := svc.MySlots(ctx, "user-sara")
	if err != nil {
		t.Fatal(err)
	}

	// Add two future slots; one duplicate start must be skipped silently.
	s1 := now.Add(48 * time.Hour)
	s2 := now.Add(49 * time.Hour)
	created, err := svc.AddSlots(ctx, "user-sara", []time.Time{s1, s2, s1}, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("created=%d want 2 (duplicate skipped)", len(created))
	}
	after, _ := svc.MySlots(ctx, "user-sara")
	if len(after) != len(before)+2 {
		t.Fatalf("slots %d -> %d, want +2", len(before), len(after))
	}

	// Overlap with an existing opening is skipped, not duplicated.
	again, err := svc.AddSlots(ctx, "user-sara", []time.Time{s1.Add(10 * time.Minute)}, 30)
	if err != nil || len(again) != 0 {
		t.Fatalf("overlap must be skipped: %v %d", err, len(again))
	}

	// Past start is a hard error; bad duration too.
	if _, err := svc.AddSlots(ctx, "user-sara", []time.Time{now.Add(-time.Hour)}, 30); err == nil {
		t.Fatal("past start must fail")
	}
	if _, err := svc.AddSlots(ctx, "user-sara", []time.Time{s1.Add(72 * time.Hour)}, 45); err == nil {
		t.Fatal("bad duration must fail")
	}

	// Booked slots can't be removed; own unbooked ones can; foreign ids 404.
	if _, err := svc.Book(ctx, "learner-1", created[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveSlot(ctx, "user-sara", created[0].ID); err == nil {
		t.Fatal("booked slot removal must fail")
	}
	if err := svc.RemoveSlot(ctx, "user-sara", created[1].ID); err != nil {
		t.Fatalf("own unbooked removal: %v", err)
	}
	if err := svc.RemoveSlot(ctx, "user-sara", "nope"); err == nil {
		t.Fatal("unknown slot must 404")
	}
}

func TestTutorCancelDeletesSlotAndNotifiesLearner(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	ctx := context.Background()
	if err := svc.LinkUser(ctx, "tutor-sara", "user-sara"); err != nil {
		t.Fatal(err)
	}

	// A learner books one of Sara's seeded slots.
	det, _ := svc.GetTutor(ctx, "tutor-sara")
	slot := det.Availability[0]
	b, err := svc.Book(ctx, "learner-9", slot.ID)
	if err != nil {
		t.Fatal(err)
	}

	// A different tutor's user cannot cancel it.
	if err := svc.LinkUser(ctx, "tutor-omid", "user-omid"); err == nil {
		if err := svc.TutorCancel(ctx, "user-omid", b.ID); err == nil {
			t.Fatal("foreign tutor cancel must fail")
		}
	}

	// The owner cancels: booking cancelled, slot GONE (not merely freed).
	if err := svc.TutorCancel(ctx, "user-sara", b.ID); err != nil {
		t.Fatal(err)
	}
	nb, _ := r.Booking(ctx, b.ID)
	if nb.Status != domain.StatusCancelled {
		t.Fatalf("status=%s", nb.Status)
	}
	if sl, _ := r.Slot(ctx, slot.ID); sl != nil {
		t.Fatal("slot must be deleted on tutor cancel")
	}
	// Idempotent second cancel.
	if err := svc.TutorCancel(ctx, "user-sara", b.ID); err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	// Non-tutor user is forbidden.
	if err := svc.TutorCancel(ctx, "random-user", b.ID); err == nil {
		t.Fatal("non-tutor must be forbidden")
	}
}

func TestTutorEditsOwnProfile(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := repo.NewMemoryTutorRepo()
	svc := NewService(r, func() time.Time { return now })
	ctx := context.Background()
	if err := svc.LinkUser(ctx, "tutor-sara", "user-sara"); err != nil {
		t.Fatal(err)
	}

	got, err := svc.UpdateMyProfile(ctx, "user-sara", UpdateMyProfileInput{
		Headline: "IELTS coach", Languages: []string{"EN ", "fa"}, HourlyGems: 80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Headline != "IELTS coach" || got.HourlyGems != 80 || got.Languages[0] != "en" {
		t.Fatalf("update wrong: %+v", got)
	}
	prevBio := got.Bio

	// Patch semantics: empty fields keep prior values.
	got2, err := svc.UpdateMyProfile(ctx, "user-sara", UpdateMyProfileInput{Bio: "New bio"})
	if err != nil {
		t.Fatal(err)
	}
	if got2.Headline != "IELTS coach" || got2.Bio != "New bio" || got2.HourlyGems != 80 {
		t.Fatalf("patch broke fields: %+v (prev bio %q)", got2, prevBio)
	}

	// Bounds + authorization.
	if _, err := svc.UpdateMyProfile(ctx, "user-sara", UpdateMyProfileInput{HourlyGems: 5000}); err == nil {
		t.Fatal("rate above cap must fail")
	}
	if _, err := svc.UpdateMyProfile(ctx, "random", UpdateMyProfileInput{Headline: "x"}); err == nil {
		t.Fatal("non-tutor must be forbidden")
	}

	// The public listing reflects the change.
	det, _ := svc.GetTutor(ctx, "tutor-sara")
	if det.Tutor.Headline != "IELTS coach" {
		t.Fatalf("public listing stale: %+v", det.Tutor.Headline)
	}
}
