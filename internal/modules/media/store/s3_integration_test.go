//go:build s3

package store

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
)

// Opt-in real S3 API check. The same fixed key can be read again after the
// isolated provider container is restarted to verify persistence.
func TestS3CompatibleProviderRoundTrip(t *testing.T) {
	endpoint := os.Getenv("LEKSUU_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set LEKSUU_S3_ENDPOINT for provider integration")
	}
	bucket := os.Getenv("LEKSUU_S3_BUCKET")
	s, err := NewS3(endpoint, "us-east-1", bucket, "qa", os.Getenv("LEKSUU_S3_ACCESS_KEY"),
		os.Getenv("LEKSUU_S3_SECRET_KEY"), "http://127.0.0.1:8082", false, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	exists, err := s.client.BucketExists(ctx, bucket)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}); err != nil {
			t.Fatal(err)
		}
	}
	const key = "integration/persistence-probe.txt"
	content := []byte("leksuu-s3-persistence-probe-2026-11-02")
	if os.Getenv("LEKSUU_S3_VERIFY_ONLY") != "1" {
		url, err := s.Put(ctx, key, "text/plain; charset=utf-8", content)
		if err != nil {
			t.Fatal(err)
		}
		if url != "http://127.0.0.1:8082/media/"+key {
			t.Fatalf("private URL: %s", url)
		}
	}
	contentType, err := s.Stat(ctx, key)
	if err != nil || !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("HEAD: %q %v", contentType, err)
	}
	info, err := s.client.StatObject(ctx, bucket, s.objectKey(key), minio.StatObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Metadata.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("cache control: %q", info.Metadata.Get("Cache-Control"))
	}
	blob, err := s.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Body.Close()
	actual, err := io.ReadAll(blob.Body)
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("GET mismatch: %q %v", actual, err)
	}
	if _, err := blob.Body.Seek(7, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	part := make([]byte, 4)
	if _, err := io.ReadFull(blob.Body, part); err != nil || !bytes.Equal(part, content[7:11]) {
		t.Fatalf("range mismatch: %q %v", part, err)
	}
	t.Logf("provider persistence key %s, size %d", s.objectKey(key), blob.Size)
}
