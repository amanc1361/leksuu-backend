//go:build pgx

package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/settings/domain"
)

// PostgresSettingsRepo stores settings as one JSON row (id=1).
type PostgresSettingsRepo struct{ pool *pgxpool.Pool }

func NewPostgresSettingsRepo(pool *pgxpool.Pool) *PostgresSettingsRepo {
	return &PostgresSettingsRepo{pool: pool}
}

func (r *PostgresSettingsRepo) Get(ctx context.Context) (*domain.Settings, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT data FROM app_settings WHERE id=1`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		d := domain.Defaults()
		return &d, nil
	}
	if err != nil {
		return nil, err
	}
	s := domain.Defaults() // start from defaults so new fields are populated
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *PostgresSettingsRepo) Save(ctx context.Context, s *domain.Settings) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO app_settings (id, data) VALUES (1, $1)
		 ON CONFLICT (id) DO UPDATE SET data=$1`, raw)
	return err
}

// PostgresPlanRepo persists subscription plans (migration 0037).
type PostgresPlanRepo struct{ pool *pgxpool.Pool }

func NewPostgresPlanRepo(pool *pgxpool.Pool) *PostgresPlanRepo { return &PostgresPlanRepo{pool: pool} }

const planCols = "product, id, title, amount_toman, days, position, active"

func scanPlan(row pgx.Row) (domain.Plan, error) {
	var p domain.Plan
	err := row.Scan(&p.Product, &p.ID, &p.Title, &p.AmountToman, &p.Days, &p.Position, &p.Active)
	return p, err
}

func (r *PostgresPlanRepo) List(ctx context.Context) ([]domain.Plan, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+planCols+` FROM subscription_plans ORDER BY product, position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PostgresPlanRepo) ListActiveByProduct(ctx context.Context, product string) ([]domain.Plan, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+planCols+` FROM subscription_plans WHERE product=$1 AND active ORDER BY position, id`, product)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *PostgresPlanRepo) Upsert(ctx context.Context, p domain.Plan) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO subscription_plans (product, id, title, amount_toman, days, position, active)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (product, id) DO UPDATE SET
		   title=EXCLUDED.title, amount_toman=EXCLUDED.amount_toman, days=EXCLUDED.days,
		   position=EXCLUDED.position, active=EXCLUDED.active`,
		p.Product, p.ID, p.Title, p.AmountToman, p.Days, p.Position, p.Active)
	return err
}

func (r *PostgresPlanRepo) Delete(ctx context.Context, product, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM subscription_plans WHERE product=$1 AND id=$2`, product, id)
	return err
}
