package repo

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/leksuu/platform/internal/modules/pronunciation/domain"
)

// MemoryPronRepo keeps pronunciation scores in memory (dev & tests).
type MemoryPronRepo struct {
	mu sync.Mutex
	m  map[string]map[int]domain.LineScore // userID|itemID -> line -> score
}

func NewMemoryPronRepo() *MemoryPronRepo {
	return &MemoryPronRepo{m: map[string]map[int]domain.LineScore{}}
}

func key(userID, itemID string) string { return userID + "|" + itemID }

func (r *MemoryPronRepo) Upsert(_ context.Context, userID, itemID string, line, score int) (domain.LineScore, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(userID, itemID)
	if r.m[k] == nil {
		r.m[k] = map[int]domain.LineScore{}
	}
	cur := r.m[k][line]
	prev := cur.Best
	cur.Line = line
	cur.Last = score
	cur.Attempts++
	if score > cur.Best {
		cur.Best = score
	}
	r.m[k][line] = cur
	return cur, prev, nil
}

func (r *MemoryPronRepo) ByItem(_ context.Context, userID, itemID string) ([]domain.LineScore, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lines := r.m[key(userID, itemID)]
	out := make([]domain.LineScore, 0, len(lines))
	for _, v := range lines {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out, nil
}

func (r *MemoryPronRepo) Summary(_ context.Context, userID string, masteredThreshold int) (domain.Summary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefix := userID + "|"
	var sum domain.Summary
	totalBest := 0
	for k, lines := range r.m {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		for _, ls := range lines {
			sum.Scored++
			totalBest += ls.Best
			if ls.Best >= masteredThreshold {
				sum.Mastered++
			}
		}
	}
	if sum.Scored > 0 {
		sum.AvgBest = int((float64(totalBest) / float64(sum.Scored)) + 0.5)
	}
	return sum, nil
}
