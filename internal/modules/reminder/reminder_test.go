package reminder

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeSource struct{ rems []Reminder }

func (f fakeSource) DueReminders(_ context.Context, _, _ time.Time) ([]Reminder, error) {
	return f.rems, nil
}

type fakeNotifier struct {
	mu    sync.Mutex
	calls []string // "userID:type"
}

func (n *fakeNotifier) Notify(_ context.Context, userID, ntype string, _ map[string]string, _ string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, userID+":"+ntype)
	return nil
}

func TestSchedulerFiresOncePerSession(t *testing.T) {
	now := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	notif := &fakeNotifier{}

	booking := fakeSource{rems: []Reminder{{
		Key: "bk-1", Type: TypeClassReminder, Link: "/room/bk-1",
		Targets: []Target{
			{UserID: "learner-1", Data: map[string]string{"name": "Sara"}},
			{UserID: "tutor-1", Data: map[string]string{"name": "Ali"}},
		},
	}}}
	class := fakeSource{rems: []Reminder{{
		Key: "cl-1", Type: TypeClassReminder, Link: "/room/cl-1",
		Targets: []Target{{UserID: "tutor-1", Data: nil}, {UserID: "stu-2", Data: nil}},
	}}}

	s := NewScheduler(notif, []Source{booking, class}, 15*time.Minute, time.Minute, func() time.Time { return now })

	// First tick: every target of every session is notified once.
	s.tick(context.Background(), now)
	if len(notif.calls) != 4 {
		t.Fatalf("first tick notifies = %d, want 4 (%v)", len(notif.calls), notif.calls)
	}

	// Second tick (same sessions still in window): deduped, no new notifications.
	s.tick(context.Background(), now.Add(time.Minute))
	if len(notif.calls) != 4 {
		t.Fatalf("second tick should be deduped, got %d (%v)", len(notif.calls), notif.calls)
	}

	// Dedupe cleanup after >2h lets a stale key fire again (e.g. a rescheduled id).
	s.tick(context.Background(), now.Add(3*time.Hour))
	if len(notif.calls) != 8 {
		t.Fatalf("after cleanup re-fire = %d, want 8", len(notif.calls))
	}
}
