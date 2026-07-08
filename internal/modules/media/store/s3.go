//go:build s3

package store

import (
	"bytes"
	"context"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3 stores objects in any S3-compatible bucket (ArvanCloud, MinIO, AWS). Build
// with `-tags s3`. Public delivery is straight from the bucket/CDN, so no API
// fan-out — set MEDIA_PUBLIC_URL to the bucket's public base.
type S3 struct {
	client     *minio.Client
	bucket     string
	publicBase string
}

func NewS3(endpoint, region, bucket, accessKey, secretKey, publicBase string, secure bool) (*S3, error) {
	c, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: region,
	})
	if err != nil {
		return nil, err
	}
	return &S3{client: c, bucket: bucket, publicBase: strings.TrimRight(publicBase, "/")}, nil
}

func (s *S3) Put(ctx context.Context, key, contentType string, data []byte) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType, CacheControl: "public, max-age=31536000, immutable"})
	if err != nil {
		return "", err
	}
	return s.publicBase + "/" + key, nil
}

// Remove deletes a stored object by key.
func (s *S3) Remove(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
