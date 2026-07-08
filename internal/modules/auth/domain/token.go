package domain

import "time"

// RefreshToken is an opaque, server-revocable credential; only its hash is stored.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	FamilyID  string // rotation lineage; reuse of a rotated token burns the family
	DeviceID  string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (t *RefreshToken) IsRevoked() bool              { return t.RevokedAt != nil }
func (t *RefreshToken) IsExpired(now time.Time) bool { return now.After(t.ExpiresAt) }
