// Package repo implements the gamification repository. memory.go backs dev/tests;
// postgres.go (build tag pgx) backs production against user_gamification + daily_goals.
package repo

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/leksuu/platform/internal/modules/gamification/domain"
)

type MemoryRepo struct {
	mu     sync.RWMutex
	states map[string]domain.State
	goals  map[string]domain.DailyGoal // key: userID|YYYY-MM-DD
	recaps map[string]bool             // key: userID|weekKey
	events map[string]bool             // key: userID|eventKey
}

func NewMemoryRepo() *MemoryRepo {
	return &MemoryRepo{states: map[string]domain.State{}, goals: map[string]domain.DailyGoal{}, recaps: map[string]bool{}, events: map[string]bool{}}
}

// Mutate applies a change under the repo lock (atomic for this process).
func (r *MemoryRepo) Mutate(_ context.Context, userID string, ch domain.Change) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ek := userID + "|" + ch.EventKey
	if ch.EventKey != "" && r.events[ek] {
		return false, nil
	}
	st, ok := r.states[userID]
	if !ok {
		st = domain.State{UserID: userID, League: domain.LeagueBronze}
	}
	if err := ch.Apply(&st); err != nil {
		return false, err
	}
	r.states[userID] = st
	if ch.DailyXP != 0 {
		target := st.DailyTargetXP
		if target <= 0 {
			target = ch.DefaultTarget
		}
		k := key(userID, ch.Day)
		g, ok := r.goals[k]
		if !ok {
			g = domain.DailyGoal{Date: ch.Day, TargetXP: target}
		}
		g.EarnedXP += ch.DailyXP
		g.Met = g.EarnedXP >= g.TargetXP
		r.goals[k] = g
	}
	if ch.EventKey != "" {
		r.events[ek] = true
	}
	return true, nil
}

func key(userID string, d time.Time) string { return userID + "|" + d.Format("2006-01-02") }

func (r *MemoryRepo) Get(_ context.Context, userID string) (*domain.State, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.states[userID]; ok {
		cp := s
		return &cp, nil
	}
	return &domain.State{UserID: userID, League: domain.LeagueBronze}, nil
}

func (r *MemoryRepo) Upsert(_ context.Context, s *domain.State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.states[s.UserID] = *s
	return nil
}

func (r *MemoryRepo) DailyGoal(_ context.Context, userID string, date time.Time, defaultTarget int) (*domain.DailyGoal, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if g, ok := r.goals[key(userID, date)]; ok {
		cp := g
		return &cp, nil
	}
	return &domain.DailyGoal{Date: date, TargetXP: defaultTarget}, nil
}

func (r *MemoryRepo) AddDailyXP(_ context.Context, userID string, date time.Time, defaultTarget, xp int) (*domain.DailyGoal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(userID, date)
	g, ok := r.goals[k]
	if !ok {
		g = domain.DailyGoal{Date: date, TargetXP: defaultTarget}
	}
	g.EarnedXP += xp
	g.Met = g.EarnedXP >= g.TargetXP
	r.goals[k] = g
	return &g, nil
}

func (r *MemoryRepo) WeeklyTotals(_ context.Context, from, to time.Time) ([]domain.WeeklyEntry, error) {
	r.mu.RLock()
	totals := map[string]int{}
	for k, g := range r.goals {
		if g.Date.Before(from) || !g.Date.Before(to) {
			continue
		}
		uid := k[:len(k)-len("|2006-01-02")]
		totals[uid] += g.EarnedXP
	}
	r.mu.RUnlock()
	out := make([]domain.WeeklyEntry, 0, len(totals))
	for uid, xp := range totals {
		if xp > 0 {
			out = append(out, domain.WeeklyEntry{UserID: uid, XP: xp})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].XP != out[j].XP {
			return out[i].XP > out[j].XP
		}
		return out[i].UserID < out[j].UserID
	})
	return out, nil
}

func (r *MemoryRepo) ActiveDays(_ context.Context, userID string, from, to time.Time) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for k, g := range r.goals {
		if g.EarnedXP <= 0 {
			continue
		}
		var uid, day string
		if i := len(k) - 10 - 1; i > 0 {
			uid, day = k[:i], k[i+1:]
		}
		if uid != userID {
			continue
		}
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			continue
		}
		if !d.Before(from) && d.Before(to) {
			n++
		}
	}
	return n, nil
}

func (r *MemoryRepo) RecapSent(_ context.Context, userID, weekKey string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.recaps[userID+"|"+weekKey], nil
}

func (r *MemoryRepo) MarkRecapSent(_ context.Context, userID, weekKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recaps[userID+"|"+weekKey] = true
	return nil
}
