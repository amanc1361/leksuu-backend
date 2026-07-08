module github.com/leksuu/platform

// Targets the latest Go (1.26.x, June 2026). The standard-library-only default
// build keeps it dependency-free; the pgx PostgreSQL adapter (build tag `pgx`)
// pulls jackc/pgx via `go mod tidy` on first build.
go 1.26.0

toolchain go1.26.4
