package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/leksuu/platform/internal/modules/notification/domain"
	"github.com/leksuu/platform/internal/modules/notification/repo"
)

func TestNotifyListReadAndHub(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	tick := 0
	clock := func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Second) }
	hub := NewHub()
	svc := NewService(repo.NewMemoryNotificationRepo(), hub, clock)
	ctx := context.Background()

	// A live subscriber receives published notifications.
	ch, cancel := hub.Subscribe("user-1")
	defer cancel()

	if err := svc.Notify(ctx, "user-1", domain.TypeAssignmentGraded, map[string]string{"score": "90"}, "/homework"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Notify(ctx, "user-1", domain.TypeBookingNew, map[string]string{"learner_name": "Ali"}, "/teach"); err != nil {
		t.Fatal(err)
	}
	// Notifications for a different user don't leak.
	_ = svc.Notify(ctx, "user-2", domain.TypeReviewNew, nil, "")

	select {
	case n := <-ch:
		if n.Type != domain.TypeAssignmentGraded {
			t.Fatalf("first live event = %s", n.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("expected a live event")
	}

	got, err := svc.List(ctx, "user-1", 10, 0)
	if err != nil || len(got) != 2 {
		t.Fatalf("list = %d (%v)", len(got), err)
	}
	// Newest first.
	if got[0].Type != domain.TypeBookingNew {
		t.Fatalf("order wrong: %s", got[0].Type)
	}

	if n, _ := svc.UnreadCount(ctx, "user-1"); n != 2 {
		t.Fatalf("unread = %d", n)
	}
	if err := svc.MarkRead(ctx, "user-1", got[0].ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.UnreadCount(ctx, "user-1"); n != 1 {
		t.Fatalf("after read unread = %d", n)
	}
	if err := svc.MarkAllRead(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.UnreadCount(ctx, "user-1"); n != 0 {
		t.Fatalf("after read-all unread = %d", n)
	}
	// Empty userID is a no-op.
	if err := svc.Notify(ctx, "", "x", nil, ""); err != nil {
		t.Fatal(err)
	}
}
