package usecase

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/leksuu/platform/internal/modules/media/store"
)

func countFiles(t *testing.T, dir string) (files []string) {
	t.Helper()
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			files = append(files, rel)
		}
		return nil
	})
	return files
}

// The same remote asset referenced by several exercises, imported again by a
// second run, must be one physical object.
func TestImportRemoteSecondRunWritesNoNewObject(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.NewLocal(dir, "https://leksuu.test")
	svc := NewService(st, 1<<20)
	fetch := func(context.Context) ([]byte, string, error) { return []byte("ID3-audio"), "audio/mpeg", nil }
	urls := []string{"https://cdn.busuu.com/a/1.mp3", "https://cdn.busuu.com/a/1.mp3", "https://cdn.busuu.com/a/2.mp3"}
	for run := 1; run <= 2; run++ {
		reused := 0
		for _, u := range urls {
			up, err := svc.ImportRemote(context.Background(), u, filepath.Base(u), fetch)
			if err != nil {
				t.Fatal(err)
			}
			if up.Reused {
				reused++
			}
		}
		want := map[int]int{1: 1, 2: 3}[run] // run 1: the repeated URL is reused once
		if reused != want {
			t.Fatalf("run %d: reused=%d want %d", run, reused, want)
		}
		if files := countFiles(t, dir); len(files) != 2 {
			t.Fatalf("run %d: physical objects = %v, want 2", run, files)
		}
	}
}

// An interrupted or failed copy must not leave anything a retry would take
// for a complete object.
func TestImportRemoteFailedFetchStoresNothing(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.NewLocal(dir, "https://leksuu.test")
	svc := NewService(st, 1<<20)
	src := "https://cdn.busuu.com/v/clip.mp4"
	_, err := svc.ImportRemote(context.Background(), src, "clip.mp4", func(context.Context) ([]byte, string, error) {
		return nil, "", errors.New("connection reset")
	})
	if err == nil {
		t.Fatal("expected the fetch error")
	}
	if files := countFiles(t, dir); len(files) != 0 {
		t.Fatalf("failed copy left %v", files)
	}
	up, err := svc.ImportRemote(context.Background(), src, "clip.mp4", func(context.Context) ([]byte, string, error) {
		return []byte("mp4-bytes"), "video/mp4", nil
	})
	if err != nil || up.Reused {
		t.Fatalf("retry must copy for real: reused=%v err=%v", up != nil && up.Reused, err)
	}
	if files := countFiles(t, dir); len(files) != 1 {
		t.Fatalf("after retry: %v (temp files must not remain)", files)
	}
}

func TestArchiveJSONReusesIdenticalSource(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.NewLocal(dir, "https://leksuu.test")
	svc := NewService(st, 1<<20)
	doc := []byte(`{"id":"objective_1","class":"unit"}`)
	first, err := svc.ArchiveJSON(context.Background(), "raw_lessons__objective_1.json", doc)
	if err != nil || first.Reused {
		t.Fatalf("first: %+v err=%v", first, err)
	}
	second, err := svc.ArchiveJSON(context.Background(), "raw_lessons__objective_1.json", doc)
	if err != nil || !second.Reused || second.StorageKey != first.StorageKey || second.SHA256 != first.SHA256 {
		t.Fatalf("second: %+v err=%v", second, err)
	}
	changed, _ := svc.ArchiveJSON(context.Background(), "raw_lessons__objective_1.json", []byte(`{"id":"objective_1","class":"unit","v":2}`))
	if changed.Reused || changed.StorageKey == first.StorageKey {
		t.Fatal("changed source must be archived as a new object")
	}
	if files := countFiles(t, dir); len(files) != 2 {
		t.Fatalf("objects = %v, want 2", files)
	}
}
