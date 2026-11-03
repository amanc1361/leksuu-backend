//go:build pgx

package repo

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This test uses a session-local temporary table; it cannot modify application
// data. Set LEKSUU_TEST_DATABASE_URL to an isolated PostgreSQL instance.
func TestPostgresTimingPayloadCompareAndSwap(t *testing.T) {
	dsn := os.Getenv("LEKSUU_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEKSUU_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1 // TEMP table and all operations must use one session.
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE TEMP TABLE lesson_items (id text PRIMARY KEY, payload jsonb NOT NULL, media_key text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	before := json.RawMessage(`{"source_id":"source-1","published":false,"transcript":[{"text":"The boat moved.","translation":{"fa":"قایق حرکت کرد.","ckb":"بەلەمەکە جووڵا."}}]}`)
	after := json.RawMessage(`{"source_id":"source-1","published":false,"transcript":[{"text":"The boat moved.","translation":{"fa":"قایق حرکت کرد.","ckb":"بەلەمەکە جووڵا."},"start":1.25,"end":2.50}]}`)
	if _, err := pool.Exec(ctx, `INSERT INTO lesson_items (id,payload,media_key) VALUES ('story-1',$1::jsonb,'original.mp3')`, []byte(before)); err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresCourseRepo(pool)
	updated, err := repository.UpdateItemPayloadIfUnchanged(ctx, "story-1", before, after)
	if err != nil || !updated {
		t.Fatalf("first timing update: updated=%t error=%v", updated, err)
	}
	updated, err = repository.UpdateItemPayloadIfUnchanged(ctx, "story-1", before, json.RawMessage(`{"source_id":"wrong"}`))
	if err != nil || updated {
		t.Fatalf("stale timing update: updated=%t error=%v", updated, err)
	}
	var stored json.RawMessage
	var mediaKey string
	if err := pool.QueryRow(ctx, `SELECT payload,media_key FROM lesson_items WHERE id='story-1'`).Scan(&stored, &mediaKey); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(stored, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(after, &want); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) || mediaKey != "original.mp3" {
		t.Fatalf("payload or media key changed unexpectedly: payload=%s key=%s", stored, mediaKey)
	}
}
