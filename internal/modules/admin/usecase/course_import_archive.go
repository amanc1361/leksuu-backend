package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

type ArchiveCourseImportInput struct {
	FileName string
	RawJSON  []byte
	Metadata json.RawMessage
}

var safeImportName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func importFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = safeImportName.ReplaceAllString(strings.TrimSpace(name), "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		return "source.json"
	}
	return name
}

func (s *Service) ArchiveCourseImportSource(ctx context.Context, in ArchiveCourseImportInput) (*coursedomain.ImportSource, error) {
	if s.store == nil || len(in.RawJSON) == 0 || !json.Valid(in.RawJSON) {
		return nil, apperr.ErrBadRequest
	}
	name := importFileName(in.FileName)
	sum := sha256.Sum256(in.RawJSON)
	digest := hex.EncodeToString(sum[:])
	key := "course-imports/" + digest[:24] + "-" + name
	url, err := s.store.Put(ctx, key, "application/json", in.RawJSON)
	if err != nil {
		return nil, apperr.ErrInternal
	}
	meta := in.Metadata
	if len(meta) == 0 || !json.Valid(meta) {
		meta = json.RawMessage(`{}`)
	}
	src := &coursedomain.ImportSource{
		FileName: name, StorageKey: key, URL: url, ContentType: "application/json",
		ContentBytes: len(in.RawJSON), SHA256: digest, Metadata: meta,
	}
	if err := s.repo.SaveImportSource(ctx, src); err != nil {
		return nil, err
	}
	return src, nil
}

func (s *Service) CourseImportSources(ctx context.Context, limit, offset int) ([]coursedomain.ImportSource, int, error) {
	return s.repo.ListImportSources(ctx, limit, offset)
}
