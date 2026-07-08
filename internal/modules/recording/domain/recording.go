// Package domain defines the class-recording bounded context. Recording follows
// Option A of the recording design: the host's browser records a composite and
// uploads it in chunks; the server appends chunks to a temp file (no transcode,
// negligible CPU) and, on completion, stores one webm object via the media
// store, served like any other media file.
package domain

import (
	"context"
	"time"
)

const (
	StatusRecording = "recording" // chunks still arriving
	StatusReady     = "ready"     // finalized + stored
)

// Recording is one captured session for a class.
type Recording struct {
	ID        string     `json:"id"`
	ClassID   string     `json:"class_id"`
	HostID    string     `json:"host_id"`
	Status    string     `json:"status"`
	SizeBytes int64      `json:"size_bytes"`
	URL       string     `json:"url"`
	Key       string     `json:"key"`
	CreatedAt time.Time  `json:"created_at"`
	ReadyAt   *time.Time `json:"ready_at,omitempty"`
}

// Repository persists recording metadata.
type Repository interface {
	Create(ctx context.Context, r *Recording) error
	Get(ctx context.Context, id string) (*Recording, error)
	Update(ctx context.Context, r *Recording) error
	ListByClass(ctx context.Context, classID string) ([]Recording, error)
	ListAll(ctx context.Context) ([]Recording, error)
	Delete(ctx context.Context, id string) error
}

// ClassAuth authorizes recording actions against a class.
type ClassAuth interface {
	Access(ctx context.Context, classID, userID string) (isHost bool, canView bool, err error)
}

// BlobStore stores the finalized recording object (satisfied by the media store).
type BlobStore interface {
	Put(ctx context.Context, key, contentType string, data []byte) (url string, err error)
}

// BlobRemover deletes a stored object. The media store implements it; the
// service type-asserts for it so deletion frees real storage when available.
type BlobRemover interface {
	Remove(ctx context.Context, key string) error
}
