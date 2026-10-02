// Package domain defines the media bounded context's ports. Store is the
// object-storage abstraction: a local-disk impl backs dev, an S3-compatible
// impl (ArvanCloud / MinIO) backs production — selected at the composition root.
package domain

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned by Reader.Get when the key holds no object.
var ErrNotFound = errors.New("media: object not found")

// Store persists an object under a key and returns its public URL.
type Store interface {
	Put(ctx context.Context, key, contentType string, data []byte) (url string, err error)
}

// LocalDir is implemented by stores that serve their own bytes from disk, so the
// API can expose them at GET /media/{key}.
type LocalDir interface {
	Dir() string
}

// Blob is one stored object opened for reading. Body is seekable so the API can
// answer a Range request — seeking inside a recording — by fetching just that
// range instead of buffering the whole object.
type Blob struct {
	Body        io.ReadSeekCloser
	ContentType string
	Size        int64
	ModTime     time.Time
}

// Reader is implemented by stores whose bytes the API has to fetch itself: a
// bucket that stays private, with no anonymous read access. The API serves it at
// GET /media/{key} — the same path a local-disk store uses — so stored URLs look
// identical whichever backend is configured.
type Reader interface {
	Get(ctx context.Context, key string) (*Blob, error)
}

// Lookup is implemented by stores that can tell whether a key already holds an
// object, and what its URL is, without writing. Remote imports use it to skip
// downloading and re-uploading media that an earlier import already copied.
type Lookup interface {
	// Stat returns the stored content type, or ErrNotFound.
	Stat(ctx context.Context, key string) (contentType string, err error)
	URL(key string) string
}
