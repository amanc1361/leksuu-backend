package usecase

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/auth/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

// Config carries tunables for the auth use case (injected, not hardcoded).
type Config struct {
	RefreshTTL time.Duration
}

// Service implements the authentication use cases.
type Service struct {
	users       domain.UserRepository
	tokens      domain.TokenRepository
	hasher      Hasher
	issuer      AccessTokenIssuer
	refresh     RefreshTokenFactory
	clock       Clock
	cfg         Config
	adminEmails map[string]bool
	subLookup   SubscriptionLookup
	onChange    func(userID string)
}

// OnAccountChange registers a callback run after a user's status or roles
// change, so cached authorization state (the request authenticator) is
// dropped immediately.
func (s *Service) OnAccountChange(f func(userID string)) { s.onChange = f }

func (s *Service) changed(userID string) {
	if s.onChange != nil {
		s.onChange(userID)
	}
}

// AccountState reports whether a user may use the API and their current
// roles. A missing user is simply inactive.
func (s *Service) AccountState(ctx context.Context, userID string) (bool, []string, error) {
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}
	if !u.IsActive() {
		return false, nil, nil
	}
	roles, err := s.users.Roles(ctx, userID)
	if err != nil {
		return false, nil, err
	}
	return true, roles, nil
}

// SubscriptionLookup enriches the admin user list with active products per user.
type SubscriptionLookup interface {
	UserSubscriptionStatuses(ctx context.Context, userIDs []string) (map[string][]string, error)
	ActiveSubscriberIDs(ctx context.Context) ([]string, error)
}

// SetSubscriptionLookup wires the subscription-status source (optional; nil-safe).
func (s *Service) SetSubscriptionLookup(l SubscriptionLookup) { s.subLookup = l }

func NewService(
	users domain.UserRepository,
	tokens domain.TokenRepository,
	hasher Hasher,
	issuer AccessTokenIssuer,
	refresh RefreshTokenFactory,
	clock Clock,
	cfg Config,
	adminEmails []string,
) *Service {
	set := make(map[string]bool, len(adminEmails))
	for _, e := range adminEmails {
		set[e] = true
	}
	return &Service{users: users, tokens: tokens, hasher: hasher, issuer: issuer,
		refresh: refresh, clock: clock, cfg: cfg, adminEmails: set}
}

// ensureAdmin grants the admin role to configured bootstrap emails (idempotent).
func (s *Service) ensureAdmin(ctx context.Context, userID, email string) {
	if s.adminEmails[email] {
		_ = s.users.AssignRole(ctx, userID, domain.RoleAdmin)
	}
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

type RegisterInput struct {
	Email, Password, DisplayName, NativeLang string
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	email := normalizeEmail(in.Email)
	if email == "" {
		return nil, domain.ErrEmailRequired
	}
	if !validEmail(email) {
		return nil, domain.ErrEmailInvalid
	}
	if in.Password == "" {
		return nil, domain.ErrPasswordRequired
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, apperr.ErrInternal
	}
	nativeLang := in.NativeLang
	if nativeLang == "" {
		nativeLang = domain.DefaultNativeLang
	}
	u := &domain.User{
		Email:        email,
		PasswordHash: hash,
		DisplayName:  in.DisplayName,
		NativeLang:   nativeLang,
		TargetLang:   domain.DefaultTargetLang,
		CEFR:         domain.DefaultCEFR,
		Status:       domain.StatusActive,
		CreatedAt:    s.clock(),
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	if err := s.users.AssignRole(ctx, u.ID, domain.RoleStudent); err != nil {
		return nil, apperr.ErrInternal
	}
	s.ensureAdmin(ctx, u.ID, u.Email)
	u.PasswordHash = ""
	return u, nil
}

func (s *Service) Login(ctx context.Context, email, password, deviceID string) (*TokenPair, error) {
	u, err := s.users.ByEmail(ctx, normalizeEmail(email))
	if err != nil || !u.IsActive() || !s.hasher.Verify(u.PasswordHash, password) {
		return nil, domain.ErrInvalidCredentials
	}
	s.ensureAdmin(ctx, u.ID, u.Email)
	return s.issue(ctx, u.ID, id.New(), deviceID)
}

func (s *Service) Refresh(ctx context.Context, rawRefresh, deviceID string) (*TokenPair, error) {
	rt, err := s.tokens.ByHash(ctx, s.refresh.Hash(rawRefresh))
	if err != nil {
		return nil, domain.ErrTokenInvalid
	}
	if rt.IsRevoked() {
		_ = s.tokens.RevokeFamily(ctx, rt.FamilyID)
		return nil, domain.ErrTokenReuse
	}
	if rt.IsExpired(s.clock()) {
		return nil, domain.ErrTokenInvalid
	}
	// A suspended (or deleted) account cannot mint new tokens: its session
	// family is revoked so the refresh token is dead for good.
	if u, err := s.users.ByID(ctx, rt.UserID); err != nil || !u.IsActive() {
		_ = s.tokens.RevokeFamily(ctx, rt.FamilyID)
		return nil, domain.ErrTokenInvalid
	}
	if err := s.tokens.Revoke(ctx, rt.ID); err != nil {
		return nil, apperr.ErrInternal
	}
	return s.issue(ctx, rt.UserID, rt.FamilyID, deviceID)
}

func (s *Service) Logout(ctx context.Context, rawRefresh string) error {
	rt, err := s.tokens.ByHash(ctx, s.refresh.Hash(rawRefresh))
	if err != nil {
		return nil
	}
	return s.tokens.Revoke(ctx, rt.ID)
}

// UpdateProfileInput carries the fields a user may change about themselves.
type UpdateProfileInput struct {
	DisplayName string
	NativeLang  string
	TargetLang  string
	CEFR        string
}

// UpdateProfile applies non-empty fields to the user and returns the fresh profile.
func (s *Service) UpdateProfile(ctx context.Context, userID string, in UpdateProfileInput) (*domain.User, error) {
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	f := domain.ProfileFields{DisplayName: u.DisplayName, NativeLang: u.NativeLang, TargetLang: u.TargetLang, CEFR: u.CEFR}
	if in.DisplayName != "" {
		f.DisplayName = in.DisplayName
	}
	if in.NativeLang != "" {
		f.NativeLang = in.NativeLang
	}
	if in.TargetLang != "" {
		f.TargetLang = in.TargetLang
	}
	if in.CEFR != "" {
		if !validCEFR(in.CEFR) {
			return nil, domain.ErrInvalidCEFR
		}
		f.CEFR = in.CEFR
	}
	if err := s.users.UpdateProfile(ctx, userID, f); err != nil {
		return nil, err
	}
	return s.Profile(ctx, userID)
}

// ChangePassword verifies the current password and sets a new one.
func (s *Service) ChangePassword(ctx context.Context, userID, current, next string) error {
	if next == "" {
		return domain.ErrPasswordRequired
	}
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return err
	}
	if !s.hasher.Verify(u.PasswordHash, current) {
		return domain.ErrInvalidCredentials
	}
	hash, err := s.hasher.Hash(next)
	if err != nil {
		return apperr.ErrInternal
	}
	return s.users.UpdatePasswordHash(ctx, userID, hash)
}

// CreateUser provisions a new account with a password (admin-initiated, e.g.
// when creating a teacher). Mirrors Register: it hashes the password, assigns
// the default student role, and returns the new user's id.
func (s *Service) CreateUser(ctx context.Context, email, displayName, password string) (string, error) {
	u, err := s.Register(ctx, RegisterInput{Email: email, Password: password, DisplayName: displayName})
	if err != nil {
		return "", err
	}
	return u.ID, nil
}

// RemoveRole revokes a role from a user (idempotent at the repo level).
func (s *Service) RemoveRole(ctx context.Context, userID, role string) error {
	defer s.changed(userID)
	return s.users.RemoveRole(ctx, userID, role)
}

// AdminSetPassword resets a user's password without the current one (admin-only;
// callers must authorize). Used to (re)set a teacher's password.
func (s *Service) AdminSetPassword(ctx context.Context, userID, next string) error {
	if next == "" {
		return domain.ErrPasswordRequired
	}
	if _, err := s.users.ByID(ctx, userID); err != nil {
		return err
	}
	hash, err := s.hasher.Hash(next)
	if err != nil {
		return apperr.ErrInternal
	}
	return s.users.UpdatePasswordHash(ctx, userID, hash)
}

func validCEFR(c string) bool {
	switch c {
	case "A1", "A2", "B1", "B2", "C1", "C2":
		return true
	}
	return false
}

// EnsureAdmin makes sure an admin account exists for the given email. If the
// user does not exist it is created with the provided password; either way the
// admin role is granted. Idempotent — safe to run on every startup, in both
// in-memory and Postgres modes.
func (s *Service) EnsureAdmin(ctx context.Context, email, password string) error {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return nil
	}
	u, err := s.users.ByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, domain.ErrUserNotFound) {
			return err
		}
		hash, herr := s.hasher.Hash(password)
		if herr != nil {
			return apperr.ErrInternal
		}
		u = &domain.User{
			Email:        email,
			PasswordHash: hash,
			DisplayName:  "Leksuu Admin",
			NativeLang:   domain.DefaultNativeLang,
			TargetLang:   domain.DefaultTargetLang,
			CEFR:         domain.DefaultCEFR,
			Status:       domain.StatusActive,
			CreatedAt:    s.clock(),
		}
		if cerr := s.users.Create(ctx, u); cerr != nil {
			return cerr
		}
		_ = s.users.AssignRole(ctx, u.ID, domain.RoleStudent)
	}
	return s.users.AssignRole(ctx, u.ID, domain.RoleAdmin)
}

// Profile returns the user behind an authenticated request (for /me).
// AssignRole grants a role to a user (idempotent at the repo level).
func (s *Service) AssignRole(ctx context.Context, userID, role string) error {
	defer s.changed(userID)
	return s.users.AssignRole(ctx, userID, role)
}

// UserAdminView is a user row for the admin console: identity, roles and active
// subscription products.
type UserAdminView struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	DisplayName   string    `json:"display_name"`
	Roles         []string  `json:"roles"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	Subscriptions []string  `json:"subscriptions"`
}

// ListUsers returns a searchable, role-filtered, paginated user list with roles
// and active subscriptions, for admin management.
func (s *Service) ListUsers(ctx context.Context, query, role, subscription, sort string, limit, offset int) ([]UserAdminView, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	// Resolve the subscription filter into an id-set constraint (cross-module).
	var subMode string
	var subIDs []string
	if (subscription == "active" || subscription == "none") && s.subLookup != nil {
		if ids, err := s.subLookup.ActiveSubscriberIDs(ctx); err == nil {
			subIDs = ids
			if subscription == "active" {
				subMode = "in"
			} else {
				subMode = "notin"
			}
		}
	}
	users, total, err := s.users.ListUsers(ctx, query, role, subMode, subIDs, sort, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	subs := map[string][]string{}
	if s.subLookup != nil {
		if m, err := s.subLookup.UserSubscriptionStatuses(ctx, ids); err == nil {
			subs = m
		}
	}
	out := make([]UserAdminView, 0, len(users))
	for _, u := range users {
		roles, _ := s.users.Roles(ctx, u.ID)
		out = append(out, UserAdminView{
			ID: u.ID, Email: u.Email, DisplayName: u.DisplayName,
			Roles: roles, Status: u.Status, CreatedAt: u.CreatedAt,
			Subscriptions: subs[u.ID],
		})
	}
	return out, total, nil
}

// UserDetailView extends the admin row with profile/language fields for the
// single-user detail page.
type UserDetailView struct {
	UserAdminView
	NativeLang string `json:"native_lang"`
	TargetLang string `json:"target_lang"`
	CEFR       string `json:"cefr"`
}

// UserDetail returns one user's full admin view (identity, roles, subscriptions).
func (s *Service) UserDetail(ctx context.Context, id string) (*UserDetailView, error) {
	u, err := s.users.ByID(ctx, id)
	if err != nil {
		return nil, apperr.New("auth.user_not_found", apperr.KindNotFound)
	}
	roles, _ := s.users.Roles(ctx, id)
	var products []string
	if s.subLookup != nil {
		if m, err := s.subLookup.UserSubscriptionStatuses(ctx, []string{id}); err == nil {
			products = m[id]
		}
	}
	return &UserDetailView{
		UserAdminView: UserAdminView{
			ID: u.ID, Email: u.Email, DisplayName: u.DisplayName,
			Roles: roles, Status: u.Status, CreatedAt: u.CreatedAt, Subscriptions: products,
		},
		NativeLang: u.NativeLang, TargetLang: u.TargetLang, CEFR: u.CEFR,
	}, nil
}

// SetUserRole toggles a manageable role (admin | teacher) on a user.
func (s *Service) SetUserRole(ctx context.Context, userID, role string, enabled bool) error {
	if role != "admin" && role != "teacher" {
		return apperr.New("auth.role_not_manageable", apperr.KindValidation)
	}
	if _, err := s.users.ByID(ctx, userID); err != nil {
		return apperr.New("auth.user_not_found", apperr.KindNotFound)
	}
	defer s.changed(userID)
	if enabled {
		return s.users.AssignRole(ctx, userID, role)
	}
	return s.users.RemoveRole(ctx, userID, role)
}

// SetUserStatus suspends or reactivates a user account (admin). A suspended user
// can no longer log in. Validates the status and the user's existence.
func (s *Service) SetUserStatus(ctx context.Context, userID, status string) error {
	if status != domain.StatusActive && status != domain.StatusSuspended {
		return apperr.New("auth.invalid_status", apperr.KindValidation)
	}
	if _, err := s.users.ByID(ctx, userID); err != nil {
		return apperr.New("auth.user_not_found", apperr.KindNotFound)
	}
	defer s.changed(userID)
	return s.users.SetStatus(ctx, userID, status)
}

// BulkSetUserRole applies a role change to many users at once. The actor's own
// admin role is never removed (skipped). Returns updated and skipped counts.
func (s *Service) BulkSetUserRole(ctx context.Context, userIDs []string, role string, enabled bool, actorID string) (updated, skipped int, err error) {
	if role != "admin" && role != "teacher" {
		return 0, 0, apperr.New("auth.role_not_manageable", apperr.KindValidation)
	}
	for _, id := range userIDs {
		if id == actorID && role == "admin" && !enabled {
			skipped++
			continue
		}
		if e := s.SetUserRole(ctx, id, role, enabled); e != nil {
			skipped++
			continue
		}
		updated++
	}
	return updated, skipped, nil
}

// UserByEmail resolves an email to a user id + display name (best-effort).
func (s *Service) UserByEmail(ctx context.Context, email string) (id, displayName string, ok bool, err error) {
	u, e := s.users.ByEmail(ctx, normalizeEmail(email))
	if e != nil || u == nil {
		return "", "", false, nil
	}
	return u.ID, u.DisplayName, true, nil
}

// DisplayNames resolves a set of user IDs to their display names (best-effort;
// missing/erroring IDs are simply omitted). Used by other modules (e.g. the
// tutor portal) to show student names without exposing the user store.
func (s *Service) DisplayNames(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, done := out[id]; done {
			continue
		}
		if u, err := s.users.ByID(ctx, id); err == nil && u != nil {
			out[id] = u.DisplayName
		}
	}
	return out, nil
}

func (s *Service) Profile(ctx context.Context, userID string) (*domain.User, error) {
	u, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u.PasswordHash = ""
	return u, nil
}

func (s *Service) issue(ctx context.Context, userID, familyID, deviceID string) (*TokenPair, error) {
	roles, err := s.users.Roles(ctx, userID)
	if err != nil {
		return nil, apperr.ErrInternal
	}
	access, expiresIn, err := s.issuer.Issue(userID, roles)
	if err != nil {
		return nil, apperr.ErrInternal
	}
	raw, hash, err := s.refresh.New()
	if err != nil {
		return nil, apperr.ErrInternal
	}
	now := s.clock()
	rt := &domain.RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		FamilyID:  familyID,
		DeviceID:  deviceID,
		ExpiresAt: now.Add(s.cfg.RefreshTTL),
		CreatedAt: now,
	}
	if err := s.tokens.Store(ctx, rt); err != nil {
		return nil, apperr.ErrInternal
	}
	return &TokenPair{AccessToken: access, RefreshToken: raw, ExpiresIn: expiresIn}, nil
}

func normalizeEmail(e string) string { return strings.TrimSpace(strings.ToLower(e)) }

// validEmail accepts a plain address ("name@example.com"): exactly what
// net/mail parses, with no display name, and a dotted domain.
func validEmail(e string) bool {
	if len(e) > 254 || strings.ContainsAny(e, " <>") {
		return false
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e {
		return false
	}
	at := strings.LastIndexByte(e, '@')
	domainPart := e[at+1:]
	return at > 0 && strings.Contains(domainPart, ".") && !strings.HasPrefix(domainPart, ".") && !strings.HasSuffix(domainPart, ".")
}
