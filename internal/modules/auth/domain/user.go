// Package domain holds the auth bounded context's entities and repository ports.
// It depends only on the shared kernel (apperr) — nothing else.
package domain

import "time"

// Role and account-status values are this context's vocabulary, declared once.
const (
	RoleStudent = "student"
	RoleTeacher = "teacher"
	RoleAdmin   = "admin"

	StatusActive    = "active"
	StatusSuspended = "suspended"
	StatusDeleted   = "deleted"

	DefaultNativeLang = "fa"
	DefaultTargetLang = "en"
	DefaultCEFR       = "A1"
)

// User is the account entity.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	NativeLang   string
	TargetLang   string
	CEFR         string
	Status       string
	CreatedAt    time.Time
}

func (u *User) IsActive() bool { return u.Status == StatusActive }

// ProfileFields are the user-editable profile attributes.
type ProfileFields struct {
	DisplayName string
	NativeLang  string
	TargetLang  string
	CEFR        string
}
