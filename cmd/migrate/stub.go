//go:build !pgx

// Stub for the default (stdlib-only) build so `go build ./...` stays green.
// The real migrator requires the database driver: build with -tags pgx.
package main

import "log"

func main() {
	log.Fatal("migrate requires the pgx build tag: go build -tags pgx ./cmd/migrate")
}
