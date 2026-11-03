package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

type TimingExceptionQueueRecord = coursedomain.TimingExceptionQueueRecord

type TimingExceptionStore interface {
	IngestTimingExceptions(context.Context, []TimingExceptionQueueRecord) (int, error)
	ListTimingExceptions(context.Context) ([]TimingExceptionQueueRecord, error)
}

type memoryTimingExceptionStore struct {
	mu      sync.Mutex
	records map[string]TimingExceptionQueueRecord
}

func NewMemoryTimingExceptionStore() TimingExceptionStore {
	return &memoryTimingExceptionStore{records: map[string]TimingExceptionQueueRecord{}}
}

func exceptionKey(record TimingExceptionQueueRecord) string {
	return record.SourceID + ":" + strconv.Itoa(record.Paragraph)
}

func (m *memoryTimingExceptionStore) IngestTimingExceptions(_ context.Context, records []TimingExceptionQueueRecord) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, record := range records {
		if previous, ok := m.records[exceptionKey(record)]; ok && previous.RecordSHA256 != record.RecordSHA256 {
			return 0, ErrStoryTimingConflict
		}
	}
	added := 0
	for _, record := range records {
		key := exceptionKey(record)
		if _, ok := m.records[key]; !ok {
			m.records[key] = record
			added++
		}
	}
	return added, nil
}

func (m *memoryTimingExceptionStore) ListTimingExceptions(_ context.Context) ([]TimingExceptionQueueRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TimingExceptionQueueRecord, 0, len(m.records))
	for _, record := range m.records {
		out = append(out, record)
	}
	return out, nil
}

func (s *Service) SetTimingExceptionStore(store TimingExceptionStore) { s.timingExceptions = store }

func textDigest(texts []string) (string, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(texts); err != nil {
		return "", err
	}
	hash := sha256.Sum256(bytes.TrimSpace(b.Bytes()))
	return hex.EncodeToString(hash[:]), nil
}

// IngestTimingExceptionReport stores evidence only. It never assigns timings,
// approves editorial decisions, or changes story payloads.
func (s *Service) IngestTimingExceptionReport(ctx context.Context, body json.RawMessage) (int, int, error) {
	if s.timingExceptions == nil {
		return 0, 0, apperr.ErrInternal
	}
	var report struct {
		Schema  string            `json:"schema"`
		Records []json.RawMessage `json:"records"`
	}
	if json.Unmarshal(body, &report) != nil || report.Schema != "leksuu.exception-review.v2" || len(report.Records) == 0 || len(report.Records) > 500 {
		return 0, 0, apperr.ErrBadRequest
	}
	items, err := s.AdminMedia(ctx, coursedomain.KindStory)
	if err != nil {
		return 0, 0, err
	}
	stories := map[string]json.RawMessage{}
	for _, item := range items {
		var identity struct {
			SourceID string `json:"source_id"`
		}
		if json.Unmarshal(item.Payload, &identity) == nil && identity.SourceID != "" {
			if _, exists := stories[identity.SourceID]; exists {
				return 0, 0, ErrStoryTimingConflict
			}
			stories[identity.SourceID] = item.Payload
		}
	}
	records := make([]TimingExceptionQueueRecord, 0, len(report.Records))
	seen := map[string]bool{}
	for _, raw := range report.Records {
		var record struct {
			SourceID         string `json:"source_id"`
			Paragraph        int    `json:"paragraph"`
			SourceText       string `json:"source_text"`
			AudioSHA256      string `json:"audio_sha256"`
			TranscriptSHA256 string `json:"transcript_sha256"`
			Category         string `json:"category"`
			Status           string `json:"status"`
		}
		if json.Unmarshal(raw, &record) != nil || record.SourceID == "" || record.Paragraph < 1 || record.Paragraph > 500 ||
			record.SourceText == "" || len(record.AudioSHA256) != 64 || len(record.TranscriptSHA256) != 64 ||
			record.Category == "" || (record.Status != "editorial_review_required" && record.Status != "confirmed_unspoken_prior_pilot") {
			return 0, 0, apperr.ErrBadRequest
		}
		key := record.SourceID + ":" + strconv.Itoa(record.Paragraph)
		if seen[key] {
			return 0, 0, apperr.ErrBadRequest
		}
		seen[key] = true
		var story struct {
			AudioSHA256 string `json:"audio_sha256"`
			Transcript  []struct {
				Text         string `json:"text"`
				TimingStatus string `json:"timing_status"`
			} `json:"transcript"`
		}
		payload, ok := stories[record.SourceID]
		if !ok || json.Unmarshal(payload, &story) != nil || record.Paragraph > len(story.Transcript) ||
			!strings.EqualFold(story.AudioSHA256, record.AudioSHA256) || story.Transcript[record.Paragraph-1].Text != record.SourceText {
			return 0, 0, ErrStoryTimingConflict
		}
		texts := make([]string, len(story.Transcript))
		for i, cue := range story.Transcript {
			texts[i] = cue.Text
		}
		hash, err := textDigest(texts)
		if err != nil {
			return 0, 0, err
		}
		if !strings.EqualFold(hash, record.TranscriptSHA256) ||
			(record.Status == "confirmed_unspoken_prior_pilot" && story.Transcript[record.Paragraph-1].TimingStatus != "not_in_audio") {
			return 0, 0, ErrStoryTimingConflict
		}
		canonical, err := json.Marshal(json.RawMessage(raw))
		if err != nil {
			return 0, 0, err
		}
		digest := sha256.Sum256(canonical)
		records = append(records, TimingExceptionQueueRecord{
			SourceID: record.SourceID, Paragraph: record.Paragraph, AudioSHA256: strings.ToLower(record.AudioSHA256),
			TranscriptSHA256: strings.ToLower(record.TranscriptSHA256), RecordSHA256: hex.EncodeToString(digest[:]), Record: canonical,
		})
	}
	added, err := s.timingExceptions.IngestTimingExceptions(ctx, records)
	return added, len(records), err
}

func (s *Service) TimingExceptionQueue(ctx context.Context) ([]json.RawMessage, error) {
	if s.timingExceptions == nil {
		return nil, apperr.ErrInternal
	}
	records, err := s.timingExceptions.ListTimingExceptions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, 0, len(records))
	for _, record := range records {
		out = append(out, record.Record)
	}
	return out, nil
}
