//go:build s3

package app

import (
	mediadomain "github.com/leksuu/platform/internal/modules/media/domain"
	mediastore "github.com/leksuu/platform/internal/modules/media/store"
	"github.com/leksuu/platform/internal/platform/config"
)

// newMediaStore (build tag s3) uses S3-compatible object storage when configured
// (ArvanCloud / MinIO / AWS), else falls back to local disk.
func newMediaStore(cfg config.Config) (mediadomain.Store, error) {
	if cfg.S3Bucket != "" {
		return mediastore.NewS3(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3Prefix, cfg.S3AccessKey, cfg.S3SecretKey, cfg.MediaPublicURL, cfg.S3Secure, cfg.S3PublicRead)
	}
	return mediastore.NewLocal(cfg.MediaDir, cfg.MediaPublicURL)
}
