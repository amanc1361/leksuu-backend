package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/classroom/repo"
)

type fakeTutors map[string]bool

func (f fakeTutors) IsTutor(_ context.Context, id string) (bool, error) { return f[id], nil }

func TestGroupClassLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo.NewMemoryClassRepo(), fakeTutors{"tutor-1": true}, nil, func() time.Time { return now })
	ctx := context.Background()
	future := now.Add(24 * time.Hour)

	// Non-tutor cannot create.
	if _, err := svc.Create(ctx, "stu-1", CreateInput{Title: "x", StartUTC: future}); err == nil {
		t.Fatal("non-tutor must not create a class")
	}
	// Past start rejected.
	if _, err := svc.Create(ctx, "tutor-1", CreateInput{Title: "x", StartUTC: now.Add(-time.Hour)}); err == nil {
		t.Fatal("past start must be rejected")
	}

	c, err := svc.Create(ctx, "tutor-1", CreateInput{Title: "Conversation hour", StartUTC: future, Capacity: 2})
	if err != nil {
		t.Fatal(err)
	}
	if c.RoomID == "" || c.Capacity != 2 {
		t.Fatalf("bad class: %+v", c)
	}

	// Two students enroll (capacity 2).
	if err := svc.Enroll(ctx, "stu-1", c.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Enroll(ctx, "stu-2", c.ID); err != nil {
		t.Fatal(err)
	}
	// Duplicate enroll rejected.
	if err := svc.Enroll(ctx, "stu-1", c.ID); err == nil {
		t.Fatal("duplicate enroll must fail")
	}
	// Over capacity rejected.
	if err := svc.Enroll(ctx, "stu-3", c.ID); err == nil {
		t.Fatal("over-capacity enroll must fail")
	}

	// The room is joinable within its time window.
	now = future
	// CanJoin: tutor + enrolled students yes, stranger no.
	for _, u := range []string{"tutor-1", "stu-1", "stu-2"} {
		if ok, room, _ := svc.CanJoin(ctx, u, c.ID); !ok || room != c.RoomID {
			t.Fatalf("%s should join", u)
		}
	}
	if ok, _, _ := svc.CanJoin(ctx, "stu-3", c.ID); ok {
		t.Fatal("stranger must not join")
	}

	// Browse shows the class with enrolled=2/full; MyClasses shows it for stu-1.
	bv, _ := svc.Browse(ctx, "stu-1")
	if len(bv) != 1 || bv[0].Enrolled != 2 || !bv[0].IsFull || !bv[0].Joined {
		t.Fatalf("browse = %+v", bv)
	}
	mv, _ := svc.MyClasses(ctx, "stu-2")
	if len(mv) != 1 || mv[0].ID != c.ID {
		t.Fatalf("my classes = %+v", mv)
	}

	// Leaving frees a seat.
	if err := svc.Leave(ctx, "stu-2", c.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Enroll(ctx, "stu-3", c.ID); err != nil {
		t.Fatalf("seat should be free: %v", err)
	}
}

func TestGroupRoomJoinWindow(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	now := base
	svc := NewService(repo.NewMemoryClassRepo(), fakeTutors{"tutor-1": true}, nil, func() time.Time { return now })
	ctx := context.Background()
	start := base.Add(2 * time.Hour)

	c, err := svc.Create(ctx, "tutor-1", CreateInput{Title: "Talk", StartUTC: start, DurationMin: 30, Capacity: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Enroll(ctx, "stu-1", c.ID); err != nil {
		t.Fatal(err)
	}

	// Well before the window (default 10 min): early, and not joinable.
	now = start.Add(-30 * time.Minute)
	info, err := svc.Room(ctx, "stu-1", c.ID)
	if err != nil || info.JoinState != "early" {
		t.Fatalf("want early, got %+v (err %v)", info, err)
	}
	if ok, _, _ := svc.CanJoin(ctx, "stu-1", c.ID); ok {
		t.Fatal("must not join 30m early")
	}
	// Inside the window: open + joinable.
	now = start.Add(-5 * time.Minute)
	if info, _ := svc.Room(ctx, "stu-1", c.ID); info.JoinState != "open" {
		t.Fatalf("want open, got %s", info.JoinState)
	}
	if ok, _, _ := svc.CanJoin(ctx, "stu-1", c.ID); !ok {
		t.Fatal("must join 5m before")
	}
	// After end: finished.
	now = start.Add(40 * time.Minute)
	if info, _ := svc.Room(ctx, "stu-1", c.ID); info.JoinState != "finished" {
		t.Fatalf("want finished, got %s", info.JoinState)
	}
	// A non-enrolled stranger is refused room info.
	if _, err := svc.Room(ctx, "stranger", c.ID); err == nil {
		t.Fatal("stranger must not get room info")
	}
}

func TestClassStats(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo.NewMemoryClassRepo(), fakeTutors{"tutor-1": true}, nil, func() time.Time { return now })
	ctx := context.Background()

	// Two classes later today, one tomorrow.
	if _, err := svc.Create(ctx, "tutor-1", CreateInput{Title: "T1", StartUTC: now.Add(2 * time.Hour), Capacity: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "tutor-1", CreateInput{Title: "T2", StartUTC: now.Add(5 * time.Hour), Capacity: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "tutor-1", CreateInput{Title: "Tomorrow", StartUTC: now.Add(26 * time.Hour), Capacity: 5}); err != nil {
		t.Fatal(err)
	}

	stats, err := svc.ClassStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 3 || stats.Today != 2 || stats.Upcoming != 3 {
		t.Fatalf("stats wrong: %+v", stats)
	}
}

type fakeSessions []time.Time

func (f fakeSessions) SessionTimes(_ context.Context) ([]time.Time, error) { return f, nil }

// ClassStats must count 1:1 live sessions alongside group classes so "classes"
// means the same thing platform-wide (admin overview).
func TestClassStatsIncludesSessions(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo.NewMemoryClassRepo(), fakeTutors{"tutor-1": true}, nil, func() time.Time { return now })
	ctx := context.Background()

	// Group classes: one today (upcoming), one tomorrow.
	if _, err := svc.Create(ctx, "tutor-1", CreateInput{Title: "G-today", StartUTC: now.Add(2 * time.Hour), Capacity: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "tutor-1", CreateInput{Title: "G-tomorrow", StartUTC: now.Add(26 * time.Hour), Capacity: 5}); err != nil {
		t.Fatal(err)
	}

	// 1:1 sessions: today+upcoming, today+past, tomorrow+upcoming.
	svc.SetSessionSource(fakeSessions{
		now.Add(3 * time.Hour),
		now.Add(-1 * time.Hour),
		now.Add(30 * time.Hour),
	})

	stats, err := svc.ClassStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// group {total 2, today 1, upcoming 2} + sessions {total 3, today 2, upcoming 2}
	if stats.Total != 5 || stats.Today != 3 || stats.Upcoming != 4 {
		t.Fatalf("combined stats wrong: %+v (want total=5 today=3 upcoming=4)", stats)
	}
}
