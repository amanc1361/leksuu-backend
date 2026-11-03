package usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

const storyImportSchema = "leksuu.story-import.v1"

type StoryImportIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type StoryImportItem struct {
	SourceID     string             `json:"source_id"`
	Title        string             `json:"title"`
	Position     int                `json:"position"`
	PartNumber   int                `json:"part_number,omitempty"`
	Transcript   int                `json:"transcript_lines"`
	TimedCues    int                `json:"timed_cues"`
	HasCover     bool               `json:"has_cover"`
	Translations map[string]int     `json:"translation_lines"`
	AudioPath    string             `json:"audio_path,omitempty"`
	AudioBytes   int64              `json:"audio_bytes,omitempty"`
	Action       string             `json:"action"`
	Valid        bool               `json:"valid"`
	Issues       []StoryImportIssue `json:"issues"`
	item         storyArchiveItem
}

type StoryImportReport struct {
	Schema        string            `json:"schema"`
	DryRun        bool              `json:"dry_run"`
	Published     bool              `json:"published"`
	Total         int               `json:"total"`
	Valid         int               `json:"valid"`
	Invalid       int               `json:"invalid"`
	Created       int               `json:"created"`
	Duplicates    int               `json:"duplicates"`
	Items         []StoryImportItem `json:"items"`
	ArchiveSHA256 string            `json:"archive_sha256"`
}

type storyManifest struct {
	Schema string `json:"schema"`
	Items  []struct {
		SourceID string `json:"source_id"`
		Path     string `json:"path"`
	} `json:"items"`
}

type storyArchiveItem struct {
	Kind                  string                     `json:"kind"`
	Title                 string                     `json:"title"`
	Author                string                     `json:"author"`
	Description           string                     `json:"description"`
	Language              string                     `json:"language"`
	CEFR                  string                     `json:"cefr"`
	Cover                 string                     `json:"cover"`
	SourceID              string                     `json:"source_id"`
	SourceURL             string                     `json:"source_url"`
	SourceArticleURL      string                     `json:"source_article_url"`
	Collection            string                     `json:"collection"`
	CollectionName        string                     `json:"collection_name"`
	CollectionDescription string                     `json:"collection_description"`
	CollectionCover       string                     `json:"collection_cover"`
	SeriesCover           string                     `json:"series_cover"`
	Genre                 json.RawMessage            `json:"genre"`
	Audience              string                     `json:"audience"`
	License               json.RawMessage            `json:"license"`
	Attribution           json.RawMessage            `json:"attribution"`
	Transcript            []MediaCue                 `json:"transcript"`
	TranscriptFile        string                     `json:"transcript_file"`
	Translations          map[string]json.RawMessage `json:"translations"`
	Audio                 string                     `json:"audio"`
	Files                 struct {
		Full struct {
			Path   string `json:"path"`
			Bytes  int64  `json:"bytes"`
			SHA256 string `json:"sha256"`
		} `json:"full"`
	} `json:"_files"`
	SourceAudioURL string                     `json:"source_audio_url"`
	Review         json.RawMessage            `json:"review"`
	SeriesID       string                     `json:"series_id"`
	PartNumber     int                        `json:"part_number"`
	PartTotal      int                        `json:"part_total"`
	Position       int                        `json:"position"`
	Raw            map[string]json.RawMessage `json:"-"`
}

var partPattern = regexp.MustCompile(`(?i)(?:[,\s]+|\s*[—–-]\s*)part\s+(one|two|three|four|five|six|seven|eight|nine|ten|\d+)\s*$`)

func partOf(title string) (string, int) {
	title = strings.TrimSpace(title)
	m := partPattern.FindStringSubmatch(title)
	if len(m) == 0 {
		return strings.TrimSpace(title), 0
	}
	names := map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10}
	p := names[strings.ToLower(m[1])]
	if p == 0 {
		p, _ = strconv.Atoi(m[1])
	}
	loc := partPattern.FindStringIndex(title)
	return strings.TrimSpace(title[:loc[0]]), p
}

func seriesKey(title string) string {
	base, _ := partOf(title)
	if i := strings.Index(strings.ToLower(base), " by "); i >= 0 {
		base = base[:i]
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, base)
}

func zipName(name string) (string, bool) {
	n := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	return n, n != "." && n != ".." && !strings.HasPrefix(n, "../") && !strings.HasPrefix(n, "/")
}

func readZipFile(f *zip.File, max uint64) ([]byte, error) {
	if f.UncompressedSize64 > max {
		return nil, fmt.Errorf("entry too large")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil || uint64(len(b)) > max {
		return nil, fmt.Errorf("entry too large")
	}
	return b, nil
}

func issue(severity, code, message string) StoryImportIssue {
	return StoryImportIssue{Severity: severity, Code: code, Message: message}
}

func hasErrors(issues []StoryImportIssue) bool {
	for _, i := range issues {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}

func subtitleLines(raw string) []string {
	raw = strings.ReplaceAll(strings.TrimPrefix(raw, "\ufeff"), "\r\n", "\n")
	blocks := strings.Split(raw, "\n\n")
	var out []string
	for _, block := range blocks {
		var text []string
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "NOTE") {
			continue
		}
		start := 0
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				start = i + 1
				break
			}
		}
		for _, line := range lines[start:] {
			line = strings.TrimSpace(line)
			if line == "" || line == "WEBVTT" || regexp.MustCompile(`^\d+$`).MatchString(line) || strings.Contains(line, "-->") {
				continue
			}
			text = append(text, line)
		}
		if len(text) > 0 {
			out = append(out, strings.Join(text, " "))
		}
	}
	return out
}

func transcriptFromFile(raw string) []MediaCue {
	// Subtitle files carry real cue boundaries. Keep them; a plain transcript
	// still follows the older untimed path below.
	if timed := timedSubtitleCues(raw); len(timed) > 0 {
		return timed
	}
	if !strings.Contains(raw, "-->") {
		var cues []MediaCue
		for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
			if line = strings.TrimSpace(line); line != "" && line != "WEBVTT" {
				cues = append(cues, MediaCue{Text: line})
			}
		}
		return cues
	}
	lines := subtitleLines(raw)
	if len(lines) == 0 {
		for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
	}
	out := make([]MediaCue, len(lines))
	for i, line := range lines {
		out[i] = MediaCue{Text: line}
	}
	return out
}

func subtitleTimingValid(raw string, cues []MediaCue) bool {
	count := strings.Count(raw, "-->")
	if count == 0 {
		return true
	}
	if count != len(cues) {
		return false
	}
	for _, cue := range cues {
		if cue.Start == nil || cue.End == nil {
			return false
		}
	}
	return true
}

var subtitleTime = regexp.MustCompile(`^\s*(?:(\d{1,2}):)?([0-5]?\d):([0-5]\d)[,.](\d{1,3})\s*-->\s*(?:(\d{1,2}):)?([0-5]?\d):([0-5]\d)[,.](\d{1,3})(?:\s+(?:(?:align|position|line|size|vertical):\S+\s*)+)?$`)

func subtitleSeconds(h, m, s, ms string) float64 {
	hours, _ := strconv.Atoi(h)
	minutes, _ := strconv.Atoi(m)
	seconds, _ := strconv.Atoi(s)
	millis, _ := strconv.Atoi(ms)
	for len(ms) < 3 {
		millis *= 10
		ms += "0"
	}
	return float64(hours*3600+minutes*60+seconds) + float64(millis)/1000
}

func timedSubtitleCues(raw string) []MediaCue {
	raw = strings.ReplaceAll(strings.TrimPrefix(raw, "\ufeff"), "\r\n", "\n")
	var out []MediaCue
	for _, block := range strings.Split(raw, "\n\n") {
		var times []string
		var text []string
		lines := strings.Split(strings.TrimSpace(block), "\n")
		timeLine := -1
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if match := subtitleTime.FindStringSubmatch(line); match != nil {
				times = match[1:]
				timeLine = i
				break
			}
		}
		for _, line := range lines[timeLine+1:] {
			if line = strings.TrimSpace(line); line != "" {
				text = append(text, line)
			}
		}
		if len(times) == 8 && len(text) > 0 {
			start := subtitleSeconds(times[0], times[1], times[2], times[3])
			end := subtitleSeconds(times[4], times[5], times[6], times[7])
			out = append(out, MediaCue{Start: &start, End: &end, Text: strings.Join(text, " ")})
		}
	}
	return out
}

func decodeTranslation(raw json.RawMessage, files map[string]*zip.File) ([]string, error) {
	var lines []string
	if json.Unmarshal(raw, &lines) == nil {
		return lines, nil
	}
	var file string
	if json.Unmarshal(raw, &file) != nil || strings.TrimSpace(file) == "" {
		return nil, fmt.Errorf("translation must be an array or archive path")
	}
	name, ok := zipName(file)
	if !ok || files[name] == nil {
		return nil, fmt.Errorf("translation file %q is missing", file)
	}
	b, err := readZipFile(files[name], 8<<20)
	if err != nil {
		return nil, err
	}
	return subtitleLines(string(b)), nil
}

func existingStories(ctx context.Context, s *Service) (map[string]coursedomain.LessonItem, error) {
	out := map[string]coursedomain.LessonItem{}
	courses, err := s.repo.ListAllCourses(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range courses {
		if c.Title != MediaBucketTitle {
			continue
		}
		units, err := s.repo.Units(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		for _, u := range units {
			lessons, err := s.repo.LessonsByUnit(ctx, u.ID)
			if err != nil {
				return nil, err
			}
			for _, l := range lessons {
				items, err := s.repo.Items(ctx, l.ID)
				if err != nil {
					return nil, err
				}
				for _, it := range items {
					if it.Kind != coursedomain.KindStory {
						continue
					}
					var p struct {
						SourceID string `json:"source_id"`
					}
					if json.Unmarshal(it.Payload, &p) == nil && p.SourceID != "" {
						out[p.SourceID] = it
					}
				}
			}
		}
	}
	return out, nil
}

func parseStoryArchive(data []byte) ([]StoryImportItem, map[string]*zip.File, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(zr.File) == 0 || len(zr.File) > 5000 {
		return nil, nil, apperr.ErrBadRequest
	}
	files := make(map[string]*zip.File, len(zr.File))
	var totalUncompressed uint64
	for _, f := range zr.File {
		name, ok := zipName(f.Name)
		totalUncompressed += f.UncompressedSize64
		if !ok || files[name] != nil || totalUncompressed > 2<<30 {
			return nil, nil, apperr.ErrBadRequest
		}
		files[name] = f
	}
	manifestFile := files["manifest.json"]
	if manifestFile == nil {
		return nil, nil, apperr.ErrBadRequest
	}
	manifestBytes, err := readZipFile(manifestFile, 8<<20)
	if err != nil {
		return nil, nil, apperr.ErrBadRequest
	}
	var manifest storyManifest
	if json.Unmarshal(manifestBytes, &manifest) != nil || len(manifest.Items) == 0 {
		return nil, nil, apperr.ErrBadRequest
	}

	seen := map[string]bool{}
	items := make([]StoryImportItem, 0, len(manifest.Items))
	for idx, ref := range manifest.Items {
		itemPath := ref.Path
		if itemPath == "" {
			itemPath = "items/" + ref.SourceID + ".json"
		}
		itemPath, ok := zipName(itemPath)
		preview := StoryImportItem{SourceID: strings.TrimSpace(ref.SourceID), Position: idx + 1, Translations: map[string]int{}, Issues: []StoryImportIssue{}, Action: "create"}
		if !ok || files[itemPath] == nil {
			preview.Issues = append(preview.Issues, issue("error", "item_missing", "Story metadata file is missing."))
			items = append(items, preview)
			continue
		}
		body, err := readZipFile(files[itemPath], 8<<20)
		if err != nil {
			preview.Issues = append(preview.Issues, issue("error", "item_too_large", "Story metadata file cannot be read."))
			items = append(items, preview)
			continue
		}
		var raw map[string]json.RawMessage
		var item storyArchiveItem
		if json.Unmarshal(body, &raw) != nil || json.Unmarshal(body, &item) != nil {
			preview.Issues = append(preview.Issues, issue("error", "item_invalid_json", "Story metadata is not valid JSON."))
			items = append(items, preview)
			continue
		}
		item.Raw = raw
		if item.SourceID != "" && ref.SourceID != "" && item.SourceID != ref.SourceID {
			preview.Issues = append(preview.Issues, issue("error", "source_id_mismatch", "Manifest and item source_id values do not match."))
		}
		if item.SourceID == "" {
			item.SourceID = ref.SourceID
		}
		preview.SourceID, preview.Title, preview.item = strings.TrimSpace(item.SourceID), strings.TrimSpace(item.Title), item
		preview.HasCover = strings.TrimSpace(item.Cover) != ""
		if item.Position > 0 {
			preview.Position = item.Position
		}
		series, inferredPart := partOf(item.Title)
		if item.SeriesID == "" && inferredPart > 0 {
			item.SeriesID = series
		}
		if item.PartNumber == 0 {
			item.PartNumber = inferredPart
		}
		preview.PartNumber, preview.item = item.PartNumber, item
		if preview.SourceID == "" {
			preview.Issues = append(preview.Issues, issue("error", "source_id_missing", "source_id is required."))
		}
		if seen[preview.SourceID] {
			preview.Issues = append(preview.Issues, issue("error", "source_id_duplicate", "source_id occurs more than once in the archive."))
		}
		seen[preview.SourceID] = true
		if preview.Title == "" {
			preview.Issues = append(preview.Issues, issue("error", "title_missing", "title is required."))
		}
		if item.Kind != "" && item.Kind != "story" {
			preview.Issues = append(preview.Issues, issue("error", "kind_invalid", "kind must be story."))
		}
		if item.Language == "" {
			item.Language = "en"
			preview.item.Language = "en"
		}
		if item.Language != "en" {
			preview.Issues = append(preview.Issues, issue("error", "language_invalid", "The source transcript language must be en."))
		}
		if item.TranscriptFile != "" && len(item.Transcript) == 0 {
			name, good := zipName(item.TranscriptFile)
			if !good || files[name] == nil {
				preview.Issues = append(preview.Issues, issue("error", "transcript_file_missing", "The transcript file is missing."))
			} else if b, e := readZipFile(files[name], 8<<20); e == nil {
				item.Transcript = transcriptFromFile(string(b))
				if !subtitleTimingValid(string(b), item.Transcript) {
					preview.Issues = append(preview.Issues, issue("error", "timing_invalid", "Some subtitle cues have missing or invalid timing."))
				}
				preview.item.Transcript = item.Transcript
			}
		}
		for i := range item.Transcript {
			item.Transcript[i].Text = strings.TrimSpace(item.Transcript[i].Text)
			if item.Transcript[i].Text == "" {
				preview.Issues = append(preview.Issues, issue("error", "transcript_blank", fmt.Sprintf("Transcript line %d is blank.", i+1)))
			}
		}
		preview.item.Transcript = item.Transcript
		preview.Transcript = len(item.Transcript)
		if len(item.Transcript) == 0 {
			preview.Issues = append(preview.Issues, issue("error", "transcript_missing", "An English transcript is required."))
		} else if len(item.Transcript) == 1 {
			preview.Issues = append(preview.Issues, issue("error", "transcript_incomplete", "A one-line transcript is treated as incomplete."))
		}
		timed := 0
		previousEnd := 0.0
		for i, cue := range item.Transcript {
			if cue.Start == nil && cue.End == nil {
				continue
			}
			timed++
			if cue.Start == nil || cue.End == nil || math.IsNaN(*cue.Start) || math.IsNaN(*cue.End) || math.IsInf(*cue.Start, 0) || math.IsInf(*cue.End, 0) || *cue.Start < 0 || *cue.End <= *cue.Start || (i > 0 && *cue.Start < previousEnd) {
				preview.Issues = append(preview.Issues, issue("error", "timing_invalid", fmt.Sprintf("Transcript timing at line %d is invalid or overlaps the previous line.", i+1)))
			} else {
				previousEnd = *cue.End
			}
		}
		if timed > 0 && timed != len(item.Transcript) {
			preview.Issues = append(preview.Issues, issue("error", "timing_partial", "Every transcript line needs start and end timing when timing is supplied."))
		}
		preview.TimedCues = timed
		for _, locale := range []string{"fa", "ckb"} {
			if trRaw, exists := item.Translations[locale]; exists {
				lines, e := decodeTranslation(trRaw, files)
				if e != nil {
					preview.Issues = append(preview.Issues, issue("error", "translation_invalid", locale+": "+e.Error()))
					continue
				}
				preview.Translations[locale] = len(lines)
				if len(lines) != len(item.Transcript) {
					preview.Issues = append(preview.Issues, issue("error", "translation_alignment", fmt.Sprintf("%s has %d lines; English has %d.", locale, len(lines), len(item.Transcript))))
					continue
				}
				for i, line := range lines {
					if strings.TrimSpace(line) != "" {
						if item.Transcript[i].Translation == nil {
							item.Transcript[i].Translation = map[string]string{}
						}
						item.Transcript[i].Translation[locale] = strings.TrimSpace(line)
					}
				}
				preview.item.Transcript = item.Transcript
			} else {
				count := 0
				for _, cue := range item.Transcript {
					if strings.TrimSpace(cue.Translation[locale]) != "" {
						count++
					}
				}
				preview.Translations[locale] = count
				if count != 0 && count != len(item.Transcript) {
					preview.Issues = append(preview.Issues, issue("error", "translation_alignment", fmt.Sprintf("%s has %d translated lines; English has %d.", locale, count, len(item.Transcript))))
				}
			}
			if preview.Translations[locale] == 0 {
				preview.Issues = append(preview.Issues, issue("warning", "translation_missing", locale+" translation is not yet supplied."))
			}
		}
		audioPath := item.Audio
		if audioPath == "" {
			audioPath = item.Files.Full.Path
		}
		audioPath, ok = zipName(audioPath)
		preview.AudioPath = audioPath
		if !ok || audioPath == "." || files[audioPath] == nil {
			preview.Issues = append(preview.Issues, issue("error", "audio_missing", "An MP3 recording is required."))
		} else {
			zf := files[audioPath]
			preview.AudioBytes = int64(zf.UncompressedSize64)
			if strings.ToLower(path.Ext(audioPath)) != ".mp3" {
				preview.Issues = append(preview.Issues, issue("error", "audio_type", "Recording must be an MP3 file."))
			}
			if item.Files.Full.Bytes > 0 && item.Files.Full.Bytes != preview.AudioBytes {
				preview.Issues = append(preview.Issues, issue("error", "audio_size", "MP3 size does not match metadata."))
			}
			audio, audioErr := readZipFile(zf, 512<<20)
			if audioErr != nil || len(audio) < 3 || !(bytes.HasPrefix(audio, []byte("ID3")) || (audio[0] == 0xff && audio[1]&0xe0 == 0xe0)) {
				preview.Issues = append(preview.Issues, issue("error", "audio_invalid", "MP3 signature is invalid."))
			} else if want := strings.ToLower(strings.TrimSpace(item.Files.Full.SHA256)); want != "" {
				sum := sha256.Sum256(audio)
				if hex.EncodeToString(sum[:]) != want {
					preview.Issues = append(preview.Issues, issue("error", "audio_checksum", "MP3 checksum does not match metadata."))
				}
			}
		}
		if len(item.Attribution) == 0 || string(item.Attribution) == "null" {
			preview.Issues = append(preview.Issues, issue("warning", "attribution_missing", "Source attribution should be reviewed."))
		}
		if strings.TrimSpace(item.CEFR) == "" {
			preview.Issues = append(preview.Issues, issue("warning", "cefr_unverified", "CEFR level is not verified."))
		}
		preview.Valid = !hasErrors(preview.Issues)
		items = append(items, preview)
	}
	groups := map[string][]int{}
	for i := range items {
		if items[i].PartNumber <= 0 {
			continue
		}
		key := items[i].item.SeriesID
		if key == "" {
			key = seriesKey(items[i].Title)
		}
		groups[key] = append(groups[key], i)
	}
	for _, indexes := range groups {
		maxPart := 0
		good := map[int]bool{}
		for _, i := range indexes {
			if items[i].PartNumber > maxPart {
				maxPart = items[i].PartNumber
			}
			if !hasErrors(items[i].Issues) {
				good[items[i].PartNumber] = true
			}
		}
		var missing []string
		for part := 1; part <= maxPart; part++ {
			if !good[part] {
				missing = append(missing, strconv.Itoa(part))
			}
		}
		if len(missing) == 0 {
			continue
		}
		for _, i := range indexes {
			if hasErrors(items[i].Issues) {
				continue
			}
			items[i].Issues = append(items[i].Issues, issue("error", "multipart_incomplete", "Multipart series is missing valid part(s): "+strings.Join(missing, ", ")+"."))
			items[i].Valid = false
		}
	}
	return items, files, nil
}

func (s *Service) ImportStoryArchive(ctx context.Context, data []byte, dryRun bool) (*StoryImportReport, error) {
	if len(data) == 0 || (s.bulkMaxBytes > 0 && int64(len(data)) > s.bulkMaxBytes) {
		return nil, apperr.ErrBadRequest
	}
	items, files, err := parseStoryArchive(data)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	report := &StoryImportReport{Schema: storyImportSchema, DryRun: dryRun, Published: false, Total: len(items), Items: items, ArchiveSHA256: hex.EncodeToString(sum[:])}
	existing, err := existingStories(ctx, s)
	if err != nil {
		return nil, err
	}
	for i := range report.Items {
		p := &report.Items[i]
		if _, found := existing[p.SourceID]; found {
			p.Action = "duplicate"
			report.Duplicates++
		}
		if p.Valid {
			report.Valid++
		} else {
			report.Invalid++
			p.Action = "excluded"
		}
	}
	if dryRun {
		return report, nil
	}
	if s.store == nil {
		return nil, apperr.ErrInternal
	}
	lessonID := ""
	sort.SliceStable(report.Items, func(i, j int) bool { return report.Items[i].Position < report.Items[j].Position })
	for i := range report.Items {
		p := &report.Items[i]
		if !p.Valid || p.Action == "duplicate" {
			continue
		}
		audio, err := readZipFile(files[p.AudioPath], uint64(s.bulkMaxBytes))
		if err != nil || len(audio) < 3 || !(bytes.HasPrefix(audio, []byte("ID3")) || (audio[0] == 0xff && audio[1]&0xe0 == 0xe0)) {
			p.Valid, p.Action = false, "excluded"
			p.Issues = append(p.Issues, issue("error", "audio_invalid", "MP3 signature is invalid."))
			report.Valid--
			report.Invalid++
			continue
		}
		audioSum := sha256.Sum256(audio)
		audioSHA := hex.EncodeToString(audioSum[:])
		if want := strings.ToLower(strings.TrimSpace(p.item.Files.Full.SHA256)); want != "" && want != audioSHA {
			p.Valid, p.Action = false, "excluded"
			p.Issues = append(p.Issues, issue("error", "audio_checksum", "MP3 checksum does not match metadata."))
			report.Valid--
			report.Invalid++
			continue
		}
		key := "stories/" + p.SourceID + "-" + audioSHA[:16] + ".mp3"
		url, err := s.store.Put(ctx, key, "audio/mpeg", audio)
		if err != nil {
			return nil, err
		}
		if lessonID == "" {
			lessonID, err = s.ensureMediaLesson(ctx)
			if err != nil {
				return nil, err
			}
		}
		series, inferred := partOf(p.item.Title)
		if p.item.SeriesID != "" {
			series = p.item.SeriesID
		}
		if p.item.PartNumber == 0 {
			p.item.PartNumber = inferred
		}
		payload := map[string]any{
			"title": p.item.Title, "author": p.item.Author, "description": p.item.Description,
			"language": p.item.Language, "level": p.item.CEFR,
			"cover": p.item.Cover, "url": url, "transcript": p.item.Transcript,
			"source_id": p.item.SourceID, "source_url": firstNonEmpty(p.item.SourceURL, p.item.SourceArticleURL),
			"source_audio_url": p.item.SourceAudioURL, "collection": p.item.Collection,
			"collection_name": p.item.CollectionName, "collection_description": p.item.CollectionDescription, "collection_cover": p.item.CollectionCover,
			"series_cover": p.item.SeriesCover,
			"audience":     p.item.Audience, "genre": rawOrNil(p.item.Genre), "license": rawOrNil(p.item.License),
			"attribution": rawOrNil(p.item.Attribution), "review": rawOrNil(p.item.Review),
			"series_id": series, "part_number": p.item.PartNumber, "part_total": p.item.PartTotal,
			"collection_position": p.Position, "audio_sha256": audioSHA, "import_sha256": report.ArchiveSHA256,
			"published": false,
		}
		body, _ := json.Marshal(payload)
		if _, err := s.AddItem(ctx, lessonID, coursedomain.KindStory, body, 0, p.SourceID); err != nil {
			return nil, err
		}
		p.Action = "created_draft"
		report.Created++
	}
	return report, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func rawOrNil(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}
