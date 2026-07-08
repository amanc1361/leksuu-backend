//go:build pgx

package repo

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/notification/domain"
)

type PostgresNotificationRepo struct{ pool *pgxpool.Pool }

func NewPostgresNotificationRepo(pool *pgxpool.Pool) *PostgresNotificationRepo {
	return &PostgresNotificationRepo{pool: pool}
}

func (r *PostgresNotificationRepo) Create(ctx context.Context, n *domain.Notification) error {
	data, _ := json.Marshal(n.Data)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notifications (id, user_id, type, data, link, read, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		n.ID, n.UserID, n.Type, string(data), n.Link, n.Read, n.CreatedAt)
	return err
}

func (r *PostgresNotificationRepo) List(ctx context.Context, userID string, limit, offset int) ([]domain.Notification, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, type, data, link, read, created_at FROM notifications
		   WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Notification{}
	for rows.Next() {
		var n domain.Notification
		var data string
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &data, &n.Link, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(data), &n.Data)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *PostgresNotificationRepo) UnreadCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id=$1 AND NOT read`, userID).Scan(&n)
	return n, err
}

func (r *PostgresNotificationRepo) MarkRead(ctx context.Context, userID, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE notifications SET read=true WHERE id=$1 AND user_id=$2`, id, userID)
	return err
}

func (r *PostgresNotificationRepo) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE notifications SET read=true WHERE user_id=$1 AND NOT read`, userID)
	return err
}

func (r *PostgresNotificationRepo) GetPrefs(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx, `SELECT type, enabled FROM notification_prefs WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var typ string
		var enabled bool
		if err := rows.Scan(&typ, &enabled); err != nil {
			return nil, err
		}
		out[typ] = enabled
	}
	return out, rows.Err()
}

func (r *PostgresNotificationRepo) SetPref(ctx context.Context, userID, ntype string, enabled bool) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notification_prefs (user_id, type, enabled) VALUES ($1,$2,$3)
		 ON CONFLICT (user_id, type) DO UPDATE SET enabled=EXCLUDED.enabled`, userID, ntype, enabled)
	return err
}
