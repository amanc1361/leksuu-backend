//go:build !s3

package app

import (
	mediadomain "github.com/leksuu/platform/internal/modules/media/domain"
	mediastore "github.com/leksuu/platform/internal/modules/media/store"
	"github.com/leksuu/platform/internal/platform/config"
)

// newMediaStore (default build) uses local-disk storage served by the API.
func newMediaStore(cfg config.Config) (mediadomain.Store, error) {
	return mediastore.NewLocal(cfg.MediaDir, cfg.MediaPublicURL)
}
