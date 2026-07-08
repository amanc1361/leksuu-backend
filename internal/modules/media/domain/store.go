// Package domain defines the media bounded context's ports. Store is the
// object-storage abstraction: a local-disk impl backs dev, an S3-compatible
// impl (ArvanCloud / MinIO) backs production — selected at the composition root.
package domain

import "context"

// Store persists an object under a key and returns its public URL.
type Store interface {
	Put(ctx context.Context, key, contentType string, data []byte) (url string, err error)
}

// LocalDir is implemented by stores that serve their own bytes from disk, so the
// API can expose them at GET /media/{key}.
type LocalDir interface {
	Dir() string
}
