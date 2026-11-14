package domain

import "context"

// UserRepository abstracts user persistence (impl: repo package).
type UserRepository interface {
	OnboardingRepository
	Create(ctx context.Context, u *User) error
	ByEmail(ctx context.Context, email string) (*User, error)
	ByID(ctx context.Context, id string) (*User, error)
	Roles(ctx context.Context, userID string) ([]string, error)
	AssignRole(ctx context.Context, userID, role string) error
	RemoveRole(ctx context.Context, userID, role string) error
	ListUsers(ctx context.Context, query, role, subMode string, subscriberIDs []string, sort string, limit, offset int) ([]User, int, error)
	SetStatus(ctx context.Context, userID, status string) error
	UpdateProfile(ctx context.Context, userID string, p ProfileFields) error
	UpdatePasswordHash(ctx context.Context, userID, hash string) error
}

// TokenRepository abstracts refresh-token persistence.
type TokenRepository interface {
	Store(ctx context.Context, t *RefreshToken) error
	ByHash(ctx context.Context, hash string) (*RefreshToken, error)
	Revoke(ctx context.Context, id string) error
	RevokeFamily(ctx context.Context, familyID string) error
}
