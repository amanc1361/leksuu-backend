//go:build pgx

// Command migrate applies the SQL migrations in ./migrations (lexical order),
// tracking applied versions in a schema_migrations table. Idempotent: already
// applied versions are skipped. Build with -tags pgx.
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	dir := os.Getenv("MIGRATIONS_DIR")
	if dir == "" {
		dir = "migrations"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		log.Fatalf("ensure schema_migrations: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		log.Fatalf("scan migrations: %v", err)
	}
	sort.Strings(files)

	applied := 0
	for _, f := range files {
		version := strings.TrimSuffix(filepath.Base(f), ".up.sql")

		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists); err != nil {
			log.Fatalf("check %s: %v", version, err)
		}
		if exists {
			continue
		}

		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("read %s: %v", f, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Fatalf("begin %s: %v", version, err)
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("apply %s: %v", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback(ctx)
			log.Fatalf("record %s: %v", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			log.Fatalf("commit %s: %v", version, err)
		}
		log.Printf("applied %s", version)
		applied++
	}
	log.Printf("migrations complete: %d applied, %d total", applied, len(files))

	if seedRequested() {
		applySeeds(ctx, pool, dir)
	}
}

// seedRequested is true when invoked as `migrate seed` or with SEED=true.
func seedRequested() bool {
	if len(os.Args) > 1 && os.Args[1] == "seed" {
		return true
	}
	return os.Getenv("SEED") == "true"
}

// applySeeds runs every migrations/seeds/*.sql. Seed files are idempotent
// (fixed IDs + ON CONFLICT), so this is safe to run repeatedly.
func applySeeds(ctx context.Context, pool *pgxpool.Pool, dir string) {
	seeds, err := filepath.Glob(filepath.Join(dir, "seeds", "*.sql"))
	if err != nil {
		log.Fatalf("scan seeds: %v", err)
	}
	sort.Strings(seeds)
	for _, f := range seeds {
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("read seed %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			log.Fatalf("apply seed %s: %v", filepath.Base(f), err)
		}
		log.Printf("seeded %s", filepath.Base(f))
	}
	log.Printf("seeding complete: %d file(s)", len(seeds))
}
