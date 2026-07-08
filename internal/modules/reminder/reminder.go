// Package reminder runs a lightweight in-process scheduler that notifies
// participants shortly before a live session (1:1 booking or group class) starts.
//
// Dedupe is in-memory: each session is reminded once per process lifetime. A
// restart inside the short pre-class window could re-remind; that edge case is
// acceptable here. A multi-instance deployment would move dedupe to a shared
// store or a leader-elected ticker.
package reminder

import (
	"context"
	"sync"
	"time"
)

// TypeClassReminder mirrors notification.domain.TypeClassReminder.
const TypeClassReminder = "class_reminder"

// Target is one recipient of a reminder, with data localized client-side.
type Target struct {
	UserID string
	Data   map[string]string
}

// Reminder is one upcoming session and everyone who should be notified about it.
// Key (the session id) is used for dedupe across ticks.
type Reminder struct {
	Key     string
	Type    string
	Link    string
	Targets []Target
}

// Source produces reminders for sessions starting in the [from, to] window.
type Source interface {
	DueReminders(ctx context.Context, from, to time.Time) ([]Reminder, error)
}

// Notifier delivers a single notification (notification.Service satisfies it).
type Notifier interface {
	Notify(ctx context.Context, userID, ntype string, data map[string]string, link string) error
}

type Scheduler struct {
	sources  []Source
	notif    Notifier
	lead     time.Duration
	interval time.Duration
	clock    func() time.Time

	mu   sync.Mutex
	sent map[string]time.Time
}

// NewScheduler builds a scheduler. lead is how far ahead of start to remind;
// interval is how often to scan.
func NewScheduler(notif Notifier, sources []Source, lead, interval time.Duration, clock func() time.Time) *Scheduler {
	if clock == nil {
		clock = time.Now
	}
	if lead <= 0 {
		lead = 15 * time.Minute
	}
	if interval <= 0 {
		interval = time.Minute
	}
	return &Scheduler{
		sources:  sources,
		notif:    notif,
		lead:     lead,
		interval: interval,
		clock:    clock,
		sent:     map[string]time.Time{},
	}
}

// Start runs the scan loop until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(s.interval)
		defer t.Stop()
		s.tick(ctx, s.clock().UTC()) // run once promptly
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.tick(ctx, s.clock().UTC())
			}
		}
	}()
}

// tick scans every source for sessions starting within the lead window and
// notifies their targets once. Exported-for-test via the package-internal call.
func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	s.cleanup(now) // purge stale dedupe keys before processing this window
	from, to := now, now.Add(s.lead)
	for _, src := range s.sources {
		rs, err := src.DueReminders(ctx, from, to)
		if err != nil {
			continue
		}
		for _, rem := range rs {
			if s.alreadySent(rem.Key) {
				continue
			}
			for _, tgt := range rem.Targets {
				_ = s.notif.Notify(ctx, tgt.UserID, rem.Type, tgt.Data, rem.Link)
			}
			s.markSent(rem.Key, now)
		}
	}
}

func (s *Scheduler) alreadySent(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sent[key]
	return ok
}

func (s *Scheduler) markSent(key string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent[key] = now
}

// cleanup drops dedupe entries older than 2h so the map can't grow unbounded.
func (s *Scheduler) cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, t := range s.sent {
		if now.Sub(t) > 2*time.Hour {
			delete(s.sent, k)
		}
	}
}
