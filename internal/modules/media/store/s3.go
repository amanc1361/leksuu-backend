//go:build s3

package store

import (
	"bytes"
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/leksuu/platform/internal/modules/media/domain"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3 stores objects in any S3-compatible bucket (ArvanCloud, MinIO, AWS). Build
// with `-tags s3`.
//
// Two delivery modes, picked by publicRead:
//
//   - publicRead=true: the bucket (or its CDN) serves objects anonymously, so
//     Put hands out a bucket URL and no byte ever flows back through the API.
//     Fastest, but every object is readable by anyone holding the URL.
//   - publicRead=false (default): the bucket stays private. Put hands out an
//     API URL and the API streams the bytes itself via Get, signing each read
//     with the bucket credentials. Costs egress through the server, and is the
//     only mode in which a private bucket works at all.
type S3 struct {
	client     *minio.Client
	bucket     string
	prefix     string
	publicBase string
	publicRead bool
}

func NewS3(endpoint, region, bucket, prefix, accessKey, secretKey, publicBase string, secure, publicRead bool) (*S3, error) {
	c, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: region,
	})
	if err != nil {
		return nil, err
	}
	return &S3{
		client:     c,
		bucket:     bucket,
		prefix:     strings.Trim(strings.TrimSpace(prefix), "/"),
		publicBase: strings.TrimRight(publicBase, "/"),
		publicRead: publicRead,
	}, nil
}

// cleanKey normalises a caller-supplied key, dropping any "../" that would
// otherwise let a request escape the configured prefix.
func cleanKey(key string) string {
	return strings.TrimPrefix(path.Clean("/"+key), "/")
}

func (s *S3) objectKey(key string) string {
	clean := cleanKey(key)
	if s.prefix == "" {
		return clean
	}
	return s.prefix + "/" + clean
}

func (s *S3) Put(ctx context.Context, key, contentType string, data []byte) (string, error) {
	objectKey := s.objectKey(key)
	_, err := s.client.PutObject(ctx, s.bucket, objectKey, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType, CacheControl: "public, max-age=31536000, immutable"})
	if err != nil {
		return "", err
	}
	if s.publicRead {
		return s.publicBase + "/" + objectKey, nil
	}
	// Private bucket: the URL points back at the API, which streams the object.
	// The prefix is deliberately absent — it is storage layout, not part of the
	// key the rest of the app knows, and Get re-applies it.
	return s.publicBase + "/media/" + cleanKey(key), nil
}

// Get opens the object for reading. minio's Object turns Seek/Read into ranged
// GETs, so http.ServeContent can answer a Range request without pulling the
// whole object through the server.
func (s *S3) Get(ctx context.Context, key string) (*domain.Blob, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.objectKey(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject is lazy: no request leaves until Stat or Read, so this is where
	// a missing key or a rejected credential actually surfaces.
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &domain.Blob{
		Body:        obj,
		ContentType: info.ContentType,
		Size:        info.Size,
		ModTime:     info.LastModified,
	}, nil
}

// Remove deletes a stored object by key.
func (s *S3) Remove(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, s.objectKey(key), minio.RemoveObjectOptions{})
}
