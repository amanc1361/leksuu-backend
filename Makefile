.PHONY: run run-pg test vet fmt build migrate seed up down

# Run with in-memory repositories (no database needed).
run:
	go run ./cmd/api

# Run against PostgreSQL (needs `go mod tidy` once + a running DB).
run-pg:
	go run -tags pgx ./cmd/api

# Apply migrations to the configured DATABASE_URL.
migrate:
	go run -tags pgx ./cmd/migrate

# Apply migrations then demo content seed.
seed:
	go run -tags pgx ./cmd/migrate seed

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

build:
	go build ./...

# Full stack (db + redis + migrate + api) in Docker.
up:
	docker compose up --build

down:
	docker compose down
