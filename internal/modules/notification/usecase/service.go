// Package usecase holds notification logic: create + live-publish notifications,
// and read them back. Service.Notify is the port other modules call when events
// happen (assignment graded, booking made, etc.).
package usecase

import (
	"context"
	"time"

	"github.com/leksuu/platform/internal/modules/notification/domain"
	"github.com/leksuu/platform/internal/shared/id"
)

type Service struct {
	repo  domain.Repository
	hub   *Hub
	clock func() time.Time
}

func NewService(repo domain.Repository, hub *Hub, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, hub: hub, clock: clock}
}

func (s *Service) Hub() *Hub { return s.hub }

// KnownTypes is the full set of notification types a user can toggle in settings.
var KnownTypes = []string{
	domain.TypeAssignmentNew,
	domain.TypeAssignmentGraded,
	domain.TypeBookingNew,
	domain.TypeBookingCancelled,
	domain.TypeClassEnrolled,
	domain.TypeClassReminder,
	domain.TypeReviewNew,
	domain.TypeTutorWelcome,
}

// enabled reports whether a user wants a given type delivered (default: on).
func (s *Service) enabled(ctx context.Context, userID, ntype string) bool {
	overrides, err := s.repo.GetPrefs(ctx, userID)
	if err != nil {
		return true // never let a prefs lookup error swallow a notification
	}
	if v, ok := overrides[ntype]; ok {
		return v
	}
	return true
}

// Prefs returns the effective on/off state for every known type for a user.
func (s *Service) Prefs(ctx context.Context, userID string) (map[string]bool, error) {
	overrides, err := s.repo.GetPrefs(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(KnownTypes))
	for _, t := range KnownTypes {
		if v, ok := overrides[t]; ok {
			out[t] = v
		} else {
			out[t] = true
		}
	}
	return out, nil
}

// SetPref turns a notification type on or off for a user.
func (s *Service) SetPref(ctx context.Context, userID, ntype string, enabled bool) error {
	return s.repo.SetPref(ctx, userID, ntype, enabled)
}

// Notify creates a notification for a user and publishes it live. Best-effort:
// it never fails the caller's main operation (errors are swallowed by callers).
// Notifications of a type the user has muted are silently dropped.
func (s *Service) Notify(ctx context.Context, userID, ntype string, data map[string]string, link string) error {
	if userID == "" {
		return nil
	}
	if !s.enabled(ctx, userID, ntype) {
		return nil
	}
	n := &domain.Notification{
		ID:        id.New(),
		UserID:    userID,
		Type:      ntype,
		Data:      data,
		Link:      link,
		CreatedAt: s.clock().UTC(),
	}
	if err := s.repo.Create(ctx, n); err != nil {
		return err
	}
	if s.hub != nil {
		s.hub.Publish(userID, n)
	}
	return nil
}

func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]domain.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	return s.repo.List(ctx, userID, limit, offset)
}

func (s *Service) UnreadCount(ctx context.Context, userID string) (int, error) {
	return s.repo.UnreadCount(ctx, userID)
}

func (s *Service) MarkRead(ctx context.Context, userID, id string) error {
	return s.repo.MarkRead(ctx, userID, id)
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	return s.repo.MarkAllRead(ctx, userID)
}
