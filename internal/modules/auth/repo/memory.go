// Package repo implements the auth domain repositories. memory.go backs dev and
// tests; postgres.go (build tag pgx) backs production.
package repo

import (
	"context"
	sortPkg "sort"
	"strings"
	"sync"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/shared/id"
)

// --- users ---

type MemoryUserRepo struct {
	mu         sync.RWMutex
	byID       map[string]*domain.User
	byEmail    map[string]string
	roles      map[string][]string
	onboarding map[string]domain.OnboardingState
}

func NewMemoryUserRepo() *MemoryUserRepo {
	return &MemoryUserRepo{byID: map[string]*domain.User{}, byEmail: map[string]string{}, roles: map[string][]string{}, onboarding: map[string]domain.OnboardingState{}}
}

func (r *MemoryUserRepo) Create(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byEmail[u.Email]; exists {
		return domain.ErrEmailTaken
	}
	if u.ID == "" {
		u.ID = id.New()
	}
	cp := *u
	r.byID[u.ID] = &cp
	r.byEmail[u.Email] = u.ID
	return nil
}

func (r *MemoryUserRepo) ByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	uid, ok := r.byEmail[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	cp := *r.byID[uid]
	return &cp, nil
}

func (r *MemoryUserRepo) ByID(_ context.Context, uid string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byID[uid]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *MemoryUserRepo) Roles(_ context.Context, userID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.roles[userID]...), nil
}

func (r *MemoryUserRepo) AssignRole(_ context.Context, userID, role string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.roles[userID] {
		if existing == role {
			return nil
		}
	}
	r.roles[userID] = append(r.roles[userID], role)
	return nil
}

// --- refresh tokens ---

type MemoryTokenRepo struct {
	mu     sync.RWMutex
	byID   map[string]*domain.RefreshToken
	byHash map[string]string
}

func NewMemoryTokenRepo() *MemoryTokenRepo {
	return &MemoryTokenRepo{byID: map[string]*domain.RefreshToken{}, byHash: map[string]string{}}
}

func (r *MemoryTokenRepo) Store(_ context.Context, t *domain.RefreshToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.ID == "" {
		t.ID = id.New()
	}
	cp := *t
	r.byID[t.ID] = &cp
	r.byHash[t.TokenHash] = t.ID
	return nil
}

func (r *MemoryTokenRepo) ByHash(_ context.Context, hash string) (*domain.RefreshToken, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tid, ok := r.byHash[hash]
	if !ok {
		return nil, domain.ErrTokenInvalid
	}
	cp := *r.byID[tid]
	return &cp, nil
}

func (r *MemoryTokenRepo) Revoke(_ context.Context, tid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.byID[tid]; ok && !t.IsRevoked() {
		now := time.Now()
		t.RevokedAt = &now
	}
	return nil
}

func (r *MemoryTokenRepo) RevokeFamily(_ context.Context, familyID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, t := range r.byID {
		if t.FamilyID == familyID && !t.IsRevoked() {
			t.RevokedAt = &now
		}
	}
	return nil
}

func (r *MemoryUserRepo) UpdateProfile(_ context.Context, userID string, p domain.ProfileFields) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.DisplayName, u.NativeLang, u.TargetLang, u.CEFR = p.DisplayName, p.NativeLang, p.TargetLang, p.CEFR
	return nil
}

func (r *MemoryUserRepo) UpdatePasswordHash(_ context.Context, userID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.PasswordHash = hash
	return nil
}

func (r *MemoryUserRepo) RemoveRole(_ context.Context, userID, role string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur := r.roles[userID]
	out := cur[:0:0]
	for _, x := range cur {
		if x != role {
			out = append(out, x)
		}
	}
	r.roles[userID] = out
	return nil
}

func (r *MemoryUserRepo) ListUsers(_ context.Context, query, role, subMode string, subscriberIDs []string, sort string, limit, offset int) ([]domain.User, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	subSet := map[string]bool{}
	for _, id := range subscriberIDs {
		subSet[id] = true
	}
	var matched []domain.User
	for _, u := range r.byID {
		if q != "" && !strings.Contains(strings.ToLower(u.Email), q) && !strings.Contains(strings.ToLower(u.DisplayName), q) {
			continue
		}
		if subMode == "in" && !subSet[u.ID] {
			continue
		}
		if subMode == "notin" && subSet[u.ID] {
			continue
		}
		if role != "" {
			has := false
			for _, rr := range r.roles[u.ID] {
				if rr == role {
					has = true
					break
				}
			}
			if !has {
				continue
			}
		}
		matched = append(matched, *u)
	}
	switch sort {
	case "oldest":
		sortPkg.Slice(matched, func(i, j int) bool { return matched[i].CreatedAt.Before(matched[j].CreatedAt) })
	case "name":
		sortPkg.Slice(matched, func(i, j int) bool {
			return strings.ToLower(matched[i].DisplayName) < strings.ToLower(matched[j].DisplayName)
		})
	default: // recent
		sortPkg.Slice(matched, func(i, j int) bool { return matched[i].CreatedAt.After(matched[j].CreatedAt) })
	}
	total := len(matched)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

func (r *MemoryUserRepo) SetStatus(_ context.Context, userID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[userID]
	if !ok {
		return domain.ErrUserNotFound
	}
	u.Status = status
	return nil
}

// A missing entry belongs to a new account and is not complete. Migrated
// accounts are handled by the SQL migration in the persistent repository.
func (r *MemoryUserRepo) GetOnboarding(_ context.Context, userID string) (domain.OnboardingState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.byID[userID]; !ok {
		return domain.OnboardingState{}, domain.ErrUserNotFound
	}
	return r.onboarding[userID], nil
}

func (r *MemoryUserRepo) CompleteOnboarding(_ context.Context, userID string, answers domain.OnboardingAnswers, skipped bool) (domain.OnboardingState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[userID]; !ok {
		return domain.OnboardingState{}, domain.ErrUserNotFound
	}
	if old, ok := r.onboarding[userID]; ok && old.Completed {
		return old, nil
	}
	now := time.Now().UTC()
	state := domain.OnboardingState{Completed: true, Skipped: skipped, Answers: answers, CompletedAt: &now}
	r.onboarding[userID] = state
	return state, nil
}
