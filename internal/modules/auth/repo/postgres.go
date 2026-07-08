//go:build pgx

package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/auth/domain"
)

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}

// --- users ---

type PostgresUserRepo struct{ pool *pgxpool.Pool }

func NewPostgresUserRepo(pool *pgxpool.Pool) *PostgresUserRepo { return &PostgresUserRepo{pool: pool} }

func (r *PostgresUserRepo) Create(ctx context.Context, u *domain.User) error {
	const q = `INSERT INTO users (email, password_hash, display_name, native_lang, target_lang, cefr_level, status)
	           VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at`
	err := r.pool.QueryRow(ctx, q, u.Email, u.PasswordHash, u.DisplayName,
		u.NativeLang, u.TargetLang, u.CEFR, u.Status).Scan(&u.ID, &u.CreatedAt)
	if isUniqueViolation(err) {
		return domain.ErrEmailTaken
	}
	return err
}

func (r *PostgresUserRepo) ByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.scanOne(ctx,
		`SELECT id,email,password_hash,display_name,native_lang,target_lang,cefr_level,status,created_at
		 FROM users WHERE email=$1`, email)
}

func (r *PostgresUserRepo) ByID(ctx context.Context, id string) (*domain.User, error) {
	return r.scanOne(ctx,
		`SELECT id,email,password_hash,display_name,native_lang,target_lang,cefr_level,status,created_at
		 FROM users WHERE id=$1`, id)
}

func (r *PostgresUserRepo) scanOne(ctx context.Context, q string, arg any) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, q, arg).Scan(&u.ID, &u.Email, &u.PasswordHash,
		&u.DisplayName, &u.NativeLang, &u.TargetLang, &u.CEFR, &u.Status, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *PostgresUserRepo) Roles(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT r.name FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

func (r *PostgresUserRepo) AssignRole(ctx context.Context, userID, role string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE name=$2
		 ON CONFLICT DO NOTHING`, userID, role)
	return err
}

// --- refresh tokens ---

type PostgresTokenRepo struct{ pool *pgxpool.Pool }

func NewPostgresTokenRepo(pool *pgxpool.Pool) *PostgresTokenRepo {
	return &PostgresTokenRepo{pool: pool}
}

func (r *PostgresTokenRepo) Store(ctx context.Context, t *domain.RefreshToken) error {
	const q = `INSERT INTO refresh_tokens (user_id, token_hash, family_id, device_id, expires_at)
	           VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q, t.UserID, t.TokenHash, t.FamilyID, t.DeviceID, t.ExpiresAt).
		Scan(&t.ID, &t.CreatedAt)
}

func (r *PostgresTokenRepo) ByHash(ctx context.Context, hash string) (*domain.RefreshToken, error) {
	var t domain.RefreshToken
	var revoked *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT id,user_id,token_hash,family_id,coalesce(device_id,''),expires_at,revoked_at,created_at
		 FROM refresh_tokens WHERE token_hash=$1`, hash).
		Scan(&t.ID, &t.UserID, &t.TokenHash, &t.FamilyID, &t.DeviceID, &t.ExpiresAt, &revoked, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	t.RevokedAt = revoked
	return &t, nil
}

func (r *PostgresTokenRepo) Revoke(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL`, id)
	return err
}

func (r *PostgresTokenRepo) RevokeFamily(ctx context.Context, familyID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=now() WHERE family_id=$1 AND revoked_at IS NULL`, familyID)
	return err
}

func (r *PostgresUserRepo) UpdateProfile(ctx context.Context, userID string, p domain.ProfileFields) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET display_name=$2, native_lang=$3, target_lang=$4, cefr_level=$5, updated_at=now() WHERE id=$1`,
		userID, p.DisplayName, p.NativeLang, p.TargetLang, p.CEFR)
	return err
}

func (r *PostgresUserRepo) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET password_hash=$2, updated_at=now() WHERE id=$1`, userID, hash)
	return err
}

func (r *PostgresUserRepo) RemoveRole(ctx context.Context, userID, role string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id=$1 AND role_id=(SELECT id FROM roles WHERE name=$2)`,
		userID, role)
	return err
}

func (r *PostgresUserRepo) ListUsers(ctx context.Context, query, role, subMode string, subscriberIDs []string, sort string, limit, offset int) ([]domain.User, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if subscriberIDs == nil {
		subscriberIDs = []string{}
	}
	// Build a filtered base with optional text + role + subscription predicates.
	base := `FROM users u WHERE ($1='' OR u.email ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%')
	         AND ($2='' OR EXISTS (SELECT 1 FROM user_roles ur JOIN roles rr ON rr.id=ur.role_id WHERE ur.user_id=u.id AND rr.name=$2))
	         AND ($3='' OR ($3='in' AND u.id = ANY($4)) OR ($3='notin' AND NOT (u.id = ANY($4))))`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) `+base, query, role, subMode, subscriberIDs).Scan(&total); err != nil {
		return nil, 0, err
	}
	orderBy := "u.created_at DESC"
	switch sort {
	case "oldest":
		orderBy = "u.created_at ASC"
	case "name":
		orderBy = "lower(u.display_name) ASC"
	}
	rows, err := r.pool.Query(ctx,
		`SELECT u.id,u.email,u.password_hash,u.display_name,u.native_lang,u.target_lang,u.cefr_level,u.status,u.created_at `+
			base+` ORDER BY `+orderBy+` LIMIT $5 OFFSET $6`, query, role, subMode, subscriberIDs, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.User{}
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.NativeLang, &u.TargetLang, &u.CEFR, &u.Status, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

func (r *PostgresUserRepo) SetStatus(ctx context.Context, userID, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET status=$2, updated_at=now() WHERE id=$1`, userID, status)
	return err
}
