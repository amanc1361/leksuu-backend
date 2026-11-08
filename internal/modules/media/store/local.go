// Package store provides Store implementations. local.go is stdlib-only and
// always available; s3.go (build tag s3) backs object storage in production.
package store

import (
	"context"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/leksuu/platform/internal/modules/media/domain"
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
	// Write to a temp file and rename it into place: the key only ever holds a
	// complete object, so an import interrupted mid-copy cannot leave a
	// truncated file that a later import would mistake for an existing copy.
	tmp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return l.URL(key), nil
}

// URL is where a stored key is served from.
func (l *Local) URL(key string) string {
	return l.baseURL + "/media/" + strings.TrimPrefix(path.Clean("/"+key), "/")
}

// Stat reports whether the key already holds a file.
func (l *Local) Stat(_ context.Context, key string) (string, error) {
	clean := strings.TrimPrefix(filepath.Clean("/"+key), "/")
	fi, err := os.Stat(filepath.Join(l.dir, clean))
	if err != nil || fi.IsDir() {
		return "", domain.ErrNotFound
	}
	return mime.TypeByExtension(filepath.Ext(clean)), nil
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
