// Package usecase holds the recording lifecycle: start (host only), append
// chunks to a server-side temp file, complete (concatenate + store one webm),
// and list (host or enrolled student). No media flows through the server during
// the live class — only the host's already-encoded upload — so this adds storage
// and a light upload, never transcoding.
package usecase

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/leksuu/platform/internal/modules/recording/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

// MaxRecordingBytes caps a single recording (defensive; ~1.5GB).
const MaxRecordingBytes int64 = 1_500_000_000

type Service struct {
	repo          domain.Repository
	auth          domain.ClassAuth
	store         domain.BlobStore
	tmpDir        string
	retentionDays int
	now           func() time.Time
}

func NewService(repo domain.Repository, auth domain.ClassAuth, store domain.BlobStore, tmpDir string) *Service {
	if tmpDir == "" {
		tmpDir = filepath.Join(os.TempDir(), "leksuu-recordings")
	}
	return &Service{repo: repo, auth: auth, store: store, tmpDir: tmpDir, now: time.Now}
}

func (s *Service) partPath(recID string) string { return filepath.Join(s.tmpDir, recID+".part") }

// Start begins a recording for a class. Only the class host may record.
func (s *Service) Start(ctx context.Context, classID, hostID string) (*domain.Recording, error) {
	isHost, _, err := s.auth.Access(ctx, classID, hostID)
	if err != nil {
		return nil, err
	}
	if !isHost {
		return nil, apperr.New("recording.host_only", apperr.KindForbidden)
	}
	if err := os.MkdirAll(s.tmpDir, 0o755); err != nil {
		return nil, apperr.ErrInternal
	}
	r := &domain.Recording{
		ID:        id.New(),
		ClassID:   classID,
		HostID:    hostID,
		Status:    domain.StatusRecording,
		CreatedAt: s.now(),
	}
	if err := s.repo.Create(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// AppendChunk appends one ordered webm chunk to the recording's temp file.
func (s *Service) AppendChunk(ctx context.Context, recID, hostID string, data []byte) (int64, error) {
	r, err := s.repo.Get(ctx, recID)
	if err != nil {
		return 0, err
	}
	if r == nil {
		return 0, apperr.New("recording.not_found", apperr.KindNotFound)
	}
	if r.HostID != hostID {
		return 0, apperr.New("recording.host_only", apperr.KindForbidden)
	}
	if r.Status != domain.StatusRecording {
		return 0, apperr.New("recording.not_active", apperr.KindConflict)
	}
	if r.SizeBytes+int64(len(data)) > MaxRecordingBytes {
		return 0, apperr.New("recording.too_large", apperr.KindValidation)
	}
	f, err := os.OpenFile(s.partPath(recID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, apperr.ErrInternal
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return 0, apperr.ErrInternal
	}
	r.SizeBytes += int64(len(data))
	if err := s.repo.Update(ctx, r); err != nil {
		return 0, err
	}
	return r.SizeBytes, nil
}

// Complete finalizes the recording: read the temp file, store it as one webm
// object, mark ready, and remove the temp file.
func (s *Service) Complete(ctx context.Context, recID, hostID string) (*domain.Recording, error) {
	r, err := s.repo.Get(ctx, recID)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, apperr.New("recording.not_found", apperr.KindNotFound)
	}
	if r.HostID != hostID {
		return nil, apperr.New("recording.host_only", apperr.KindForbidden)
	}
	if r.Status == domain.StatusReady {
		return r, nil // idempotent
	}
	data, err := os.ReadFile(s.partPath(recID))
	if err != nil || len(data) == 0 {
		return nil, apperr.New("recording.empty", apperr.KindValidation)
	}
	key := "rec-" + recID + ".webm"
	url, err := s.store.Put(ctx, key, "video/webm", data)
	if err != nil {
		return nil, apperr.ErrInternal
	}
	now := s.now()
	r.Status = domain.StatusReady
	r.URL = url
	r.Key = key
	r.SizeBytes = int64(len(data))
	r.ReadyAt = &now
	if err := s.repo.Update(ctx, r); err != nil {
		return nil, err
	}
	_ = os.Remove(s.partPath(recID))
	return r, nil
}

// ListForClass returns the ready recordings for a class, for the host or an
// enrolled student.
func (s *Service) ListForClass(ctx context.Context, classID, userID string) ([]domain.Recording, error) {
	_, canView, err := s.auth.Access(ctx, classID, userID)
	if err != nil {
		return nil, err
	}
	if !canView {
		return nil, apperr.New("recording.no_access", apperr.KindForbidden)
	}
	all, err := s.repo.ListByClass(ctx, classID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Recording, 0, len(all))
	for _, r := range all {
		if r.Status == domain.StatusReady {
			out = append(out, r)
		}
	}
	return out, nil
}

// ListAll returns every recording (admin storage management).
func (s *Service) ListAll(ctx context.Context) ([]domain.Recording, error) {
	return s.repo.ListAll(ctx)
}

// DeleteRecording removes a recording's stored object, its temp part file, and
// its metadata. Admin-only (the route guards the role); frees real storage.
func (s *Service) DeleteRecording(ctx context.Context, recID string) error {
	r, err := s.repo.Get(ctx, recID)
	if err != nil {
		return err
	}
	if r == nil {
		return apperr.New("recording.not_found", apperr.KindNotFound)
	}
	if r.Key != "" {
		if remover, ok := s.store.(domain.BlobRemover); ok {
			_ = remover.Remove(ctx, r.Key) // best-effort; metadata removal still proceeds
		}
	}
	_ = os.Remove(s.partPath(recID)) // clear any unfinished temp file
	return s.repo.Delete(ctx, recID)
}

// SetRetentionDays configures auto-cleanup. 0 (default) keeps recordings forever.
func (s *Service) SetRetentionDays(d int) {
	if d < 0 {
		d = 0
	}
	s.retentionDays = d
}

// RetentionDays reports the configured retention (0 = disabled).
func (s *Service) RetentionDays() int { return s.retentionDays }

// PurgeExpired deletes recordings older than the retention window (blob +
// metadata), returning how many were removed. No-op when retention is disabled.
func (s *Service) PurgeExpired(ctx context.Context) (int, error) {
	if s.retentionDays <= 0 {
		return 0, nil
	}
	cutoff := s.now().AddDate(0, 0, -s.retentionDays)
	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, r := range all {
		if r.CreatedAt.Before(cutoff) {
			if err := s.DeleteRecording(ctx, r.ID); err == nil {
				deleted++
			}
		}
	}
	return deleted, nil
}

// StartCleanup runs PurgeExpired promptly and then on the given interval until
// ctx is cancelled. A no-op when retention is disabled.
func (s *Service) StartCleanup(ctx context.Context, interval time.Duration) {
	if s.retentionDays <= 0 {
		return
	}
	if interval <= 0 {
		interval = 12 * time.Hour
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		_, _ = s.PurgeExpired(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, _ = s.PurgeExpired(ctx)
			}
		}
	}()
}
