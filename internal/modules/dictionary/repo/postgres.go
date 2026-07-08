//go:build pgx

package repo

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/dictionary/domain"
)

type PostgresDictRepo struct{ pool *pgxpool.Pool }

func NewPostgresDictRepo(pool *pgxpool.Pool) *PostgresDictRepo { return &PostgresDictRepo{pool: pool} }

const dictCols = `id, name, url, method, COALESCE(header_name,''), COALESCE(header_value,''), COALESCE(body_template,''), COALESCE(response_path,''), enabled, position`

func scanProvider(row interface{ Scan(...any) error }) (*domain.Provider, error) {
	var p domain.Provider
	if err := row.Scan(&p.ID, &p.Name, &p.URL, &p.Method, &p.HeaderName, &p.HeaderValue, &p.BodyTemplate, &p.ResponsePath, &p.Enabled, &p.Position); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PostgresDictRepo) query(ctx context.Context, where string) ([]domain.Provider, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+dictCols+` FROM dict_providers `+where+` ORDER BY position, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Provider
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *PostgresDictRepo) List(ctx context.Context) ([]domain.Provider, error) {
	return r.query(ctx, "")
}
func (r *PostgresDictRepo) Enabled(ctx context.Context) ([]domain.Provider, error) {
	return r.query(ctx, "WHERE enabled = true")
}

func (r *PostgresDictRepo) Get(ctx context.Context, id string) (*domain.Provider, error) {
	p, err := scanProvider(r.pool.QueryRow(ctx, `SELECT `+dictCols+` FROM dict_providers WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrProviderNotFound
	}
	return p, err
}

func (r *PostgresDictRepo) Create(ctx context.Context, p *domain.Provider) error {
	return r.pool.QueryRow(ctx,
		`INSERT INTO dict_providers (name, url, method, header_name, header_value, body_template, response_path, enabled, position)
		 VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9) RETURNING id`,
		p.Name, p.URL, p.Method, p.HeaderName, p.HeaderValue, p.BodyTemplate, p.ResponsePath, p.Enabled, p.Position).Scan(&p.ID)
}

func (r *PostgresDictRepo) Update(ctx context.Context, p *domain.Provider) error {
	var id string
	err := r.pool.QueryRow(ctx,
		`UPDATE dict_providers SET name=$2, url=$3, method=$4, header_name=NULLIF($5,''), header_value=NULLIF($6,''),
		   body_template=NULLIF($7,''), response_path=NULLIF($8,''), enabled=$9, position=$10 WHERE id=$1 RETURNING id`,
		p.ID, p.Name, p.URL, p.Method, p.HeaderName, p.HeaderValue, p.BodyTemplate, p.ResponsePath, p.Enabled, p.Position).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrProviderNotFound
	}
	return err
}

func (r *PostgresDictRepo) Delete(ctx context.Context, id string) error {
	var got string
	err := r.pool.QueryRow(ctx, `DELETE FROM dict_providers WHERE id=$1 RETURNING id`, id).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrProviderNotFound
	}
	return err
}
