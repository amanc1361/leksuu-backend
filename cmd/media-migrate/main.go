//go:build s3

// Command media-migrate inventories local media and, with -apply, copies it to
// an S3-compatible bucket. It never deletes or rewrites local files or DB rows.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	mediastore "github.com/leksuu/platform/internal/modules/media/store"
	"github.com/leksuu/platform/internal/modules/media/domain"
	"github.com/leksuu/platform/internal/platform/config"
)

type record struct {
	Key string `json:"key"`
	Bytes int64 `json:"bytes"`
	SHA256 string `json:"sha256"`
	Status string `json:"status"`
	Error string `json:"error,omitempty"`
}

func digest(reader io.Reader) (string, int64, error) {
	hash := sha256.New()
	n, err := io.Copy(hash, reader)
	return hex.EncodeToString(hash.Sum(nil)), n, err
}

func main() {
	apply := flag.Bool("apply", false, "copy and verify objects; default is read-only inventory")
	manifest := flag.String("manifest", "media-migration-report.json", "output report path")
	flag.Parse()
	cfg := config.Load()
	var store *mediastore.S3
	if *apply {
		if cfg.S3Bucket == "" || cfg.S3Endpoint == "" || cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
			fmt.Fprintln(os.Stderr, "S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY and S3_SECRET_KEY are required for -apply")
			os.Exit(2)
		}
		var err error
		store, err = mediastore.NewS3(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3Prefix, cfg.S3AccessKey, cfg.S3SecretKey, cfg.MediaPublicURL, cfg.S3Secure, false)
		if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(2) }
	}
	ctx := context.Background()
	var records []record
	err := filepath.WalkDir(cfg.MediaDir, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil { return walkErr }
		if entry.IsDir() { return nil }
		rel, err := filepath.Rel(cfg.MediaDir, name)
		if err != nil { return err }
		key := filepath.ToSlash(rel)
		if strings.HasPrefix(filepath.Base(name), ".upload-") { return nil }
		file, err := os.Open(name)
		if err != nil { return err }
		sha, size, err := digest(file)
		file.Close()
		if err != nil { return err }
		rec := record{Key: key, Bytes: size, SHA256: sha, Status: "inventory"}
		if *apply {
			if blob, getErr := store.Get(ctx, key); getErr == nil {
				remoteSHA, remoteSize, readErr := digest(blob.Body)
				blob.Body.Close()
				if readErr != nil { rec.Status, rec.Error = "failed", readErr.Error() } else if remoteSHA == sha && remoteSize == size {
					rec.Status = "already_verified"
				} else { rec.Status, rec.Error = "conflict", "existing object differs; refusing overwrite" }
			} else if getErr != domain.ErrNotFound { rec.Status, rec.Error = "failed", getErr.Error() }
			if rec.Status == "inventory" {
				data, readErr := os.ReadFile(name)
				if readErr != nil { rec.Status, rec.Error = "failed", readErr.Error() } else {
					contentType := mime.TypeByExtension(filepath.Ext(name))
					if contentType == "" { contentType = "application/octet-stream" }
					if _, putErr := store.Put(ctx, key, contentType, data); putErr != nil { rec.Status, rec.Error = "failed", putErr.Error() } else if blob, getErr := store.Get(ctx, key); getErr != nil {
						rec.Status, rec.Error = "failed", getErr.Error()
					} else {
						remoteSHA, remoteSize, verifyErr := digest(blob.Body)
						blob.Body.Close()
						if verifyErr != nil || remoteSHA != sha || remoteSize != size { rec.Status, rec.Error = "failed", "uploaded object failed checksum verification" } else { rec.Status = "uploaded_verified" }
					}
				}
			}
		}
		records = append(records, rec)
		return nil
	})
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	report, err := json.MarshalIndent(map[string]any{"dry_run": !*apply, "media_dir": cfg.MediaDir, "bucket": cfg.S3Bucket, "prefix": cfg.S3Prefix, "objects": records}, "", "  ")
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	if err := os.WriteFile(*manifest, report, 0o600); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	fmt.Printf("Wrote %d object records to %s (apply=%t)\n", len(records), *manifest, *apply)
	for _, rec := range records { if rec.Status == "failed" || rec.Status == "conflict" { os.Exit(1) } }
}
