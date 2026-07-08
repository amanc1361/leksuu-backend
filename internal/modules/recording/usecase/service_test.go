package usecase_test

import (
	"context"
	"testing"

	"github.com/leksuu/platform/internal/modules/recording/repo"
	"github.com/leksuu/platform/internal/modules/recording/usecase"
)

type fakeAuth struct {
	host map[string]string // classID -> hostID
	view map[string]bool   // userID -> canView
}

func (f fakeAuth) Access(_ context.Context, classID, userID string) (bool, bool, error) {
	isHost := f.host[classID] == userID
	return isHost, isHost || f.view[userID], nil
}

type fakeStore struct {
	puts    int
	removes int
}

func (f *fakeStore) Put(_ context.Context, key, _ string, data []byte) (string, error) {
	f.puts++
	return "/media/" + key, nil
}

func (f *fakeStore) Remove(_ context.Context, _ string) error {
	f.removes++
	return nil
}

func TestRecordingLifecycle(t *testing.T) {
	ctx := context.Background()
	auth := fakeAuth{host: map[string]string{"class-1": "tutor-1"}, view: map[string]bool{"student-1": true}}
	store := &fakeStore{}
	svc := usecase.NewService(repo.NewMemoryRepo(), auth, store, t.TempDir())

	// Non-host cannot start.
	if _, err := svc.Start(ctx, "class-1", "student-1"); err == nil {
		t.Fatal("non-host must not start a recording")
	}

	rec, err := svc.Start(ctx, "class-1", "tutor-1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "recording" {
		t.Fatalf("status=%s", rec.Status)
	}

	// Append two chunks; size accumulates.
	if _, err := svc.AppendChunk(ctx, rec.ID, "tutor-1", []byte("AAAA")); err != nil {
		t.Fatal(err)
	}
	size, err := svc.AppendChunk(ctx, rec.ID, "tutor-1", []byte("BBBB"))
	if err != nil {
		t.Fatal(err)
	}
	if size != 8 {
		t.Fatalf("size=%d, want 8", size)
	}

	// A different user cannot append to someone else's recording.
	if _, err := svc.AppendChunk(ctx, rec.ID, "student-1", []byte("X")); err == nil {
		t.Fatal("non-owner must not append")
	}

	// Complete stores one object and flips to ready.
	done, err := svc.Complete(ctx, rec.ID, "tutor-1")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "ready" || done.URL == "" || store.puts != 1 {
		t.Fatalf("bad completion: %+v puts=%d", done, store.puts)
	}

	// Cannot append after completion.
	if _, err := svc.AppendChunk(ctx, rec.ID, "tutor-1", []byte("late")); err == nil {
		t.Fatal("append after complete must fail")
	}

	// List: host and enrolled student see the ready recording; outsider denied.
	for _, who := range []string{"tutor-1", "student-1"} {
		recs, err := svc.ListForClass(ctx, "class-1", who)
		if err != nil {
			t.Fatalf("list for %s: %v", who, err)
		}
		if len(recs) != 1 || recs[0].Status != "ready" {
			t.Fatalf("list for %s = %+v", who, recs)
		}
	}
	if _, err := svc.ListForClass(ctx, "class-1", "outsider"); err == nil {
		t.Fatal("outsider must not list recordings")
	}
}

func TestAdminListAndDelete(t *testing.T) {
	ctx := context.Background()
	auth := fakeAuth{host: map[string]string{"class-1": "tutor-1", "class-2": "tutor-1"}}
	store := &fakeStore{}
	svc := usecase.NewService(repo.NewMemoryRepo(), auth, store, t.TempDir())

	// Two finished recordings across two classes.
	mk := func(class string) string {
		r, err := svc.Start(ctx, class, "tutor-1")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.AppendChunk(ctx, r.ID, "tutor-1", []byte("DATA")); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Complete(ctx, r.ID, "tutor-1"); err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	id1 := mk("class-1")
	_ = mk("class-2")

	all, err := svc.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAll=%d, want 2", len(all))
	}

	// Delete one — the blob is removed and metadata is gone.
	if err := svc.DeleteRecording(ctx, id1); err != nil {
		t.Fatal(err)
	}
	if store.removes != 1 {
		t.Fatalf("store removes=%d, want 1", store.removes)
	}
	all, _ = svc.ListAll(ctx)
	if len(all) != 1 {
		t.Fatalf("after delete ListAll=%d, want 1", len(all))
	}

	// Deleting an unknown recording is a not-found error.
	if err := svc.DeleteRecording(ctx, "nope"); err == nil {
		t.Fatal("expected not_found")
	}
}

func TestPurgeExpired(t *testing.T) {
	ctx := context.Background()
	auth := fakeAuth{host: map[string]string{"class-1": "tutor-1"}}
	store := &fakeStore{}
	svc := usecase.NewService(repo.NewMemoryRepo(), auth, store, t.TempDir())

	// Disabled retention purges nothing.
	mk := func() string {
		r, _ := svc.Start(ctx, "class-1", "tutor-1")
		_, _ = svc.AppendChunk(ctx, r.ID, "tutor-1", []byte("DATA"))
		_, _ = svc.Complete(ctx, r.ID, "tutor-1")
		return r.ID
	}
	mk()
	mk()
	if n, _ := svc.PurgeExpired(ctx); n != 0 {
		t.Fatalf("disabled retention purged %d, want 0", n)
	}

	// Enable 7-day retention. Nothing is old enough yet (just created).
	svc.SetRetentionDays(7)
	if n, _ := svc.PurgeExpired(ctx); n != 0 {
		t.Fatalf("fresh recordings purged %d, want 0", n)
	}
	if all, _ := svc.ListAll(ctx); len(all) != 2 {
		t.Fatalf("ListAll=%d, want 2", len(all))
	}
}
