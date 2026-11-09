//go:build s3

package store

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/media/domain"
	"github.com/minio/minio-go/v7"
)

const multipartPartSize int64 = 16 << 20
const maxMultipartSize int64 = multipartPartSize * 10000

var ErrMultipartTicket = domain.ErrMultipartTicket
var ErrMultipartIncomplete = domain.ErrMultipartIncomplete

type multipartTicket struct {
	Key         string `json:"key"`
	UploadID    string `json:"upload_id"`
	Subject     string `json:"subject"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Expires     int64  `json:"expires"`
}

func multipartExt(contentType string) string {
	switch contentType {
	case "video/mp4", "video/x-m4v":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	case "video/x-matroska":
		return ".mkv"
	case "video/x-msvideo":
		return ".avi"
	case "video/mpeg":
		return ".mpeg"
	case "video/ogg":
		return ".ogv"
	case "audio/aac":
		return ".aac"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/opus":
		return ".opus"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/mp4", "audio/x-m4a":
		return ".m4a"
	case "audio/ogg":
		return ".ogg"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/webm":
		return ".webm"
	default:
		return ""
	}
}

func (s *S3) signMultipartTicket(ticket multipartTicket) (string, error) {
	data, err := json.Marshal(ticket)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.ticketKey)
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *S3) parseMultipartTicket(raw, subject string) (multipartTicket, error) {
	var ticket multipartTicket
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || len(raw) > 4096 || len(s.ticketKey) == 0 {
		return ticket, ErrMultipartTicket
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ticket, ErrMultipartTicket
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ticket, ErrMultipartTicket
	}
	mac := hmac.New(sha256.New, s.ticketKey)
	mac.Write(data)
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(data, &ticket) != nil ||
		ticket.Subject != subject || ticket.Expires < time.Now().Unix() || ticket.UploadID == "" ||
		!(strings.HasPrefix(ticket.Key, "large/") || strings.HasPrefix(ticket.Key, "protected/large/")) ||
		ticket.Size <= 0 || ticket.Size > maxMultipartSize ||
		multipartExt(ticket.ContentType) == "" {
		return multipartTicket{}, ErrMultipartTicket
	}
	return ticket, nil
}

func (s *S3) BeginMultipart(ctx context.Context, subject, contentType string, size int64, protected bool) (string, int64, error) {
	if subject == "" || len(s.ticketKey) == 0 || size <= 0 || size > maxMultipartSize || multipartExt(contentType) == "" {
		return "", 0, ErrMultipartTicket
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", 0, err
	}
	key := "large/" + hex.EncodeToString(random) + multipartExt(contentType)
	if protected {
		key = "protected/" + key
	}
	core := minio.Core{Client: s.client}
	id, err := core.NewMultipartUpload(ctx, s.bucket, s.objectKey(key), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", 0, err
	}
	stamp, err := s.signMultipartTicket(multipartTicket{Key: key, UploadID: id, Subject: subject, ContentType: contentType, Size: size, Expires: time.Now().Add(24 * time.Hour).Unix()})
	if err != nil {
		_ = core.AbortMultipartUpload(ctx, s.bucket, s.objectKey(key), id)
		return "", 0, err
	}
	return stamp, multipartPartSize, nil
}

func (s *S3) MultipartPartURL(ctx context.Context, raw, subject string, partNumber int) (string, error) {
	ticket, err := s.parseMultipartTicket(raw, subject)
	if err != nil {
		return "", err
	}
	count := int((ticket.Size + multipartPartSize - 1) / multipartPartSize)
	if partNumber < 1 || partNumber > count {
		return "", ErrMultipartTicket
	}
	query := url.Values{"uploadId": {ticket.UploadID}, "partNumber": {strconv.Itoa(partNumber)}}
	signed, err := s.client.Presign(ctx, http.MethodPut, s.bucket, s.objectKey(ticket.Key), 15*time.Minute, query)
	if err != nil {
		return "", err
	}
	return signed.String(), nil
}

func (s *S3) listedMultipartParts(ctx context.Context, ticket multipartTicket) ([]minio.ObjectPart, error) {
	core := minio.Core{Client: s.client}
	parts := make([]minio.ObjectPart, 0)
	marker := 0
	for {
		result, err := core.ListObjectParts(ctx, s.bucket, s.objectKey(ticket.Key), ticket.UploadID, marker, 1000)
		if err != nil {
			if minio.ToErrorResponse(err).Code == "NoSuchUpload" {
				return nil, ErrMultipartTicket
			}
			return nil, err
		}
		parts = append(parts, result.ObjectParts...)
		if !result.IsTruncated {
			break
		}
		if result.NextPartNumberMarker <= marker {
			return nil, fmt.Errorf("multipart list did not advance")
		}
		marker = result.NextPartNumberMarker
	}
	return parts, nil
}

func (s *S3) MultipartParts(ctx context.Context, raw, subject string) ([]int, error) {
	ticket, err := s.parseMultipartTicket(raw, subject)
	if err != nil {
		return nil, err
	}
	parts, err := s.listedMultipartParts(ctx, ticket)
	if err != nil {
		return nil, err
	}
	numbers := make([]int, len(parts))
	for i, part := range parts {
		numbers[i] = part.PartNumber
	}
	return numbers, nil
}

func (s *S3) CompleteMultipart(ctx context.Context, raw, subject string, partSHA256 []string) (string, string, string, error) {
	ticket, err := s.parseMultipartTicket(raw, subject)
	if err != nil {
		return "", "", "", err
	}
	parts, err := s.listedMultipartParts(ctx, ticket)
	if err != nil {
		return "", "", "", err
	}
	count := int((ticket.Size + multipartPartSize - 1) / multipartPartSize)
	if len(parts) != count || len(partSHA256) != count {
		return "", "", "", ErrMultipartIncomplete
	}
	for _, digest := range partSHA256 {
		if len(digest) != 64 {
			return "", "", "", ErrMultipartIncomplete
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return "", "", "", ErrMultipartIncomplete
		}
	}
	complete := make([]minio.CompletePart, count)
	for i, part := range parts {
		expected := multipartPartSize
		if i == count-1 {
			expected = ticket.Size - int64(i)*multipartPartSize
		}
		if part.PartNumber != i+1 || part.Size != expected || part.ETag == "" {
			return "", "", "", ErrMultipartIncomplete
		}
		complete[i] = minio.CompletePart{PartNumber: part.PartNumber, ETag: part.ETag}
	}
	core := minio.Core{Client: s.client}
	if _, err := core.CompleteMultipartUpload(ctx, s.bucket, s.objectKey(ticket.Key), ticket.UploadID, complete, minio.PutObjectOptions{}); err != nil {
		return "", "", "", err
	}
	info, err := s.client.StatObject(ctx, s.bucket, s.objectKey(ticket.Key), minio.StatObjectOptions{})
	if err != nil || info.Size != ticket.Size {
		s.cleanupCompleted(ticket.Key)
		return "", "", "", ErrMultipartIncomplete
	}
	object, err := s.client.GetObject(ctx, s.bucket, s.objectKey(ticket.Key), minio.GetObjectOptions{})
	if err != nil {
		s.cleanupCompleted(ticket.Key)
		return "", "", "", err
	}
	defer object.Close()
	full := sha256.New()
	for i := 0; i < count; i++ {
		part := sha256.New()
		n := multipartPartSize
		if i == count-1 {
			n = ticket.Size - int64(i)*multipartPartSize
		}
		if _, err = io.CopyN(io.MultiWriter(part, full), object, n); err != nil || !strings.EqualFold(hex.EncodeToString(part.Sum(nil)), partSHA256[i]) {
			s.cleanupCompleted(ticket.Key)
			return "", "", "", ErrMultipartIncomplete
		}
	}
	return ticket.Key, s.URL(ticket.Key), hex.EncodeToString(full.Sum(nil)), nil
}

// A canceled HTTP request must not cancel removal of a completed object whose
// checksum failed. The bucket lifecycle remains the fallback for process loss.
func (s *S3) cleanupCompleted(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = s.Remove(ctx, key)
}

func (s *S3) AbortMultipart(ctx context.Context, raw, subject string) error {
	ticket, err := s.parseMultipartTicket(raw, subject)
	if err != nil {
		return err
	}
	core := minio.Core{Client: s.client}
	err = core.AbortMultipartUpload(ctx, s.bucket, s.objectKey(ticket.Key), ticket.UploadID)
	if err != nil && minio.ToErrorResponse(err).Code == "NoSuchUpload" {
		return ErrMultipartTicket
	}
	return err
}
