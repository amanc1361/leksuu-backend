// Package store provides Store implementations. local.go is stdlib-only and
// always available; s3.go (build tag s3) backs object storage in production.
package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Local writes objects to a directory and serves them via the API.
type Local struct {
	dir     string
	baseURL string
}

func NewLocal(dir, baseURL string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Local{dir: dir, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

func (l *Local) Put(_ context.Context, key, _ string, data []byte) (string, error) {
	clean := strings.TrimPrefix(filepath.Clean("/"+key), "/")
	target := filepath.Join(l.dir, clean)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", err
	}
	return l.baseURL + "/media/" + clean, nil
}

// Remove deletes a stored object by key (no error if it's already gone).
func (l *Local) Remove(_ context.Context, key string) error {
	clean := strings.TrimPrefix(filepath.Clean("/"+key), "/")
	err := os.Remove(filepath.Join(l.dir, clean))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (l *Local) Dir() string { return l.dir }
