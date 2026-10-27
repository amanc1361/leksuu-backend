package repo

import (
	"context"
	"sort"
	"time"

	"github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/id"
)

func (r *MemoryCourseRepo) SaveImportSource(_ context.Context, src *domain.ImportSource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if src.ID == "" {
		src.ID = id.New()
	}
	if src.CreatedAt.IsZero() {
		src.CreatedAt = time.Now().UTC()
	}
	if len(src.Metadata) == 0 {
		src.Metadata = raw(`{}`)
	}
	for i := range r.imports {
		if r.imports[i].StorageKey == src.StorageKey {
			src.ID = r.imports[i].ID
			src.CreatedAt = r.imports[i].CreatedAt
			r.imports[i] = *src
			return nil
		}
	}
	r.imports = append(r.imports, *src)
	return nil
}

func (r *MemoryCourseRepo) ListImportSources(_ context.Context, limit, offset int) ([]domain.ImportSource, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := append([]domain.ImportSource(nil), r.imports...)
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return append([]domain.ImportSource(nil), all[offset:end]...), total, nil
}
