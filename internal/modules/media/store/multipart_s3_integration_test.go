//go:build s3

package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/leksuu/platform/internal/modules/media/domain"
)

// This isolated S3 protocol fixture exercises actual MinIO SDK HTTP calls,
// presigned PUTs and object reads without external bucket credentials.
type multipartFixture struct {
	mu      sync.Mutex
	objects map[string][]byte
	parts   map[string]map[int][]byte
}

func (f *multipartFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-amz-request-id", "fixture")
	q := r.URL.Query()
	key := r.URL.Path
	if _, ok := q["location"]; ok {
		_, _ = io.WriteString(w, "<LocationConstraint>us-east-1</LocationConstraint>")
		return
	}
	if _, ok := q["uploads"]; ok && r.Method == http.MethodPost {
		id := "upload-" + strconv.Itoa(len(f.parts)+1)
		f.parts[id] = map[int][]byte{}
		_, _ = fmt.Fprintf(w, "<InitiateMultipartUploadResult><Bucket>media</Bucket><Key>%s</Key><UploadId>%s</UploadId></InitiateMultipartUploadResult>", key, id)
		return
	}
	if id := q.Get("uploadId"); id != "" {
		parts, exists := f.parts[id]
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchUpload</Code></Error>")
			return
		}
		switch r.Method {
		case http.MethodPut:
			n, _ := strconv.Atoi(q.Get("partNumber"))
			data, _ := io.ReadAll(r.Body)
			parts[n] = data
			hash := sha256.Sum256(data)
			w.Header().Set("ETag", `"`+hex.EncodeToString(hash[:16])+`"`)
		case http.MethodGet:
			numbers := make([]int, 0, len(parts))
			for n := range parts {
				numbers = append(numbers, n)
			}
			sort.Ints(numbers)
			_, _ = fmt.Fprintf(w, "<ListPartsResult><Bucket>media</Bucket><Key>%s</Key><UploadId>%s</UploadId><IsTruncated>false</IsTruncated>", key, id)
			for _, n := range numbers {
				hash := sha256.Sum256(parts[n])
				_, _ = fmt.Fprintf(w, "<Part><PartNumber>%d</PartNumber><LastModified>2026-11-08T00:00:00.000Z</LastModified><ETag>\"%s\"</ETag><Size>%d</Size></Part>", n, hex.EncodeToString(hash[:16]), len(parts[n]))
			}
			_, _ = io.WriteString(w, "</ListPartsResult>")
		case http.MethodPost:
			numbers := make([]int, 0, len(parts))
			for n := range parts {
				numbers = append(numbers, n)
			}
			sort.Ints(numbers)
			var body []byte
			for _, n := range numbers {
				body = append(body, parts[n]...)
			}
			f.objects[key] = body
			delete(f.parts, id)
			_, _ = fmt.Fprintf(w, "<CompleteMultipartUploadResult><Bucket>media</Bucket><Key>%s</Key><ETag>\"complete\"</ETag></CompleteMultipartUploadResult>", key)
		case http.MethodDelete:
			delete(f.parts, id)
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}
	data, exists := f.objects[key]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
		return
	}
	w.Header().Set("Last-Modified", "Sun, 08 Nov 2026 00:00:00 GMT")
	switch r.Method {
	case http.MethodHead:
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("ETag", `"complete"`)
		w.Header().Set("Content-Type", "video/mp4")
	case http.MethodGet:
		_, _ = w.Write(data)
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestMultipartInterruptedResumeChecksumAndAbortHTTP(t *testing.T) {
	fixture := &multipartFixture{objects: map[string][]byte{}, parts: map[string]map[int][]byte{}}
	server := httptest.NewServer(fixture)
	defer server.Close()
	s, err := NewS3(strings.TrimPrefix(server.URL, "http://"), "us-east-1", "media", "qa", "access", "secret", "http://api.local", false, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	subject := "admin-1"
	first := bytes.Repeat([]byte("a"), int(multipartPartSize))
	second := []byte("last bytes")
	ticket, partSize, err := s.BeginMultipart(ctx, subject, "video/mp4", int64(len(first)+len(second)), true)
	if err != nil || partSize != multipartPartSize {
		t.Fatalf("begin: %v, size=%d", err, partSize)
	}
	decoded, err := s.parseMultipartTicket(ticket, subject)
	if err != nil || !strings.HasPrefix(decoded.Key, "protected/large/") {
		t.Fatalf("licensed upload was not protected: %q, %v", decoded.Key, err)
	}
	put := func(number int, data []byte) {
		u, err := s.MultipartPartURL(ctx, ticket, subject, number)
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(http.MethodPut, u, bytes.NewReader(data))
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("part %d: %v, response=%v", number, err, resp)
		}
		resp.Body.Close()
	}
	put(1, first)
	parts, err := s.MultipartParts(ctx, ticket, subject)
	if err != nil || len(parts) != 1 || parts[0] != 1 {
		t.Fatalf("interrupted status: %v %v", parts, err)
	}
	put(2, second)
	digest := func(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
	key, _, full, err := s.CompleteMultipart(ctx, ticket, subject, []string{digest(first), digest(second)})
	if err != nil || key == "" || full != digest(append(bytes.Clone(first), second...)) {
		t.Fatalf("complete: key=%q full=%q err=%v", key, full, err)
	}
	wrong, _, err := s.BeginMultipart(ctx, subject, "video/mp4", int64(len(second)), true)
	if err != nil {
		t.Fatal(err)
	}
	url, err := s.MultipartPartURL(ctx, wrong, subject, 1)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(second))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if _, _, _, err = s.CompleteMultipart(ctx, wrong, subject, []string{digest([]byte("wrong"))}); err != domain.ErrMultipartIncomplete {
		t.Fatalf("checksum mismatch: %v", err)
	}
	aborted, _, err := s.BeginMultipart(ctx, subject, "video/mp4", int64(len(second)), false)
	if err == nil {
		general, parseErr := s.parseMultipartTicket(aborted, subject)
		if parseErr != nil || !strings.HasPrefix(general.Key, "large/") {
			t.Fatalf("general upload prefix: %q, %v", general.Key, parseErr)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AbortMultipart(ctx, aborted, subject); err != nil {
		t.Fatal(err)
	}
	if _, err = s.MultipartParts(ctx, aborted, subject); err != domain.ErrMultipartTicket {
		t.Fatalf("aborted status: %v", err)
	}
}
