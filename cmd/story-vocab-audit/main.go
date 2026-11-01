// Command story-vocab-audit extracts the Teen & Adult story lexicon and checks
// it against the public vocabulary lookup API. It writes a machine-readable
// coverage report, a concise Markdown report, and an unverified import template.
// The template is intentionally not importable until a reviewer fills the
// missing meanings and explicitly sets verified=true on each reviewed row.
package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type storyItem struct {
	Title      string `json:"title"`
	Language   string `json:"language"`
	Collection string `json:"collection"`
	Audience   string `json:"audience"`
	SourceID   string `json:"source_id"`
	Transcript []struct {
		Text string `json:"text"`
	} `json:"transcript"`
}

type tokenStat struct {
	Count      int
	Lowercase  int
	CapitalMid int
	Forms      map[string]int
	Contexts   []string
}

type lemmaReport struct {
	Lemma         string            `json:"lemma"`
	Count         int               `json:"count"`
	Forms         []string          `json:"forms"`
	ProperNoun    bool              `json:"proper_noun"`
	Contexts      []string          `json:"contexts"`
	Status        string            `json:"status"`
	DatabaseLemma string            `json:"database_lemma,omitempty"`
	Meaning       map[string]string `json:"meaning,omitempty"`
	Pronunciation string            `json:"pronunciation,omitempty"`
	Example       string            `json:"dictionary_example,omitempty"`
	LookupError   string            `json:"lookup_error,omitempty"`
}

type summary struct {
	GeneratedAt         string  `json:"generated_at"`
	Archive             string  `json:"archive"`
	API                 string  `json:"api"`
	Stories             int     `json:"stories"`
	TranscriptLines     int     `json:"transcript_lines"`
	Tokens              int     `json:"tokens"`
	DistinctSurfaces    int     `json:"distinct_surfaces"`
	DistinctLemmas      int     `json:"distinct_lemmas"`
	ProperNouns         int     `json:"proper_nouns"`
	CoverageDenominator int     `json:"coverage_denominator"`
	Complete            int     `json:"complete"`
	MissingEntries      int     `json:"missing_entries"`
	MissingPersian      int     `json:"missing_persian"`
	MissingSorani       int     `json:"missing_sorani"`
	LookupErrors        int     `json:"lookup_errors"`
	CompletePercent     float64 `json:"complete_percent"`
}

type coverageReport struct {
	Summary summary       `json:"summary"`
	Lemmas  []lemmaReport `json:"lemmas"`
}

type importTemplate struct {
	Schema      string              `json:"schema"`
	GeneratedAt string              `json:"generated_at"`
	Notice      string              `json:"notice"`
	Items       []importTemplateRow `json:"items"`
}

type importTemplateRow struct {
	Language        string            `json:"language"`
	Kind            string            `json:"kind"`
	Lemma           string            `json:"lemma"`
	Meaning         map[string]string `json:"meaning"`
	ExistingMeaning map[string]string `json:"existing_meaning,omitempty"`
	Example         string            `json:"example,omitempty"`
	Source          string            `json:"source"`
	Needs           []string          `json:"needs"`
	Verified        bool              `json:"verified"`
}

var wordPattern = regexp.MustCompile(`[A-Za-z]+(?:['’][A-Za-z]+)*`)

func main() {
	archive := flag.String("archive", "", "path to the prepared story ZIP")
	api := flag.String("api", "https://leksuu.ir", "site origin or API base; empty skips lookup")
	out := flag.String("out", "story-vocab-report", "output directory")
	workers := flag.Int("concurrency", 6, "parallel API lookups")
	flag.Parse()
	if strings.TrimSpace(*archive) == "" {
		fatalf("-archive is required")
	}
	stats, stories, lines, tokens, err := extract(*archive)
	if err != nil {
		fatalf("extract stories: %v", err)
	}
	lemmas := aggregate(stats)
	if strings.TrimSpace(*api) != "" {
		lookupAll(context.Background(), *api, lemmas, max(1, *workers))
	} else {
		for i := range lemmas {
			lemmas[i].Status = "not_checked"
		}
	}
	generated := time.Now().UTC().Format(time.RFC3339)
	report := coverageReport{Summary: summarize(generated, *archive, *api, stories, lines, tokens, len(stats), lemmas), Lemmas: lemmas}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatalf("create output: %v", err)
	}
	writeJSON(filepath.Join(*out, "coverage.json"), report)
	writeJSON(filepath.Join(*out, "import-template.json"), buildTemplate(generated, lemmas))
	if err := os.WriteFile(filepath.Join(*out, "coverage.md"), []byte(markdown(report)), 0o644); err != nil {
		fatalf("write markdown: %v", err)
	}
	fmt.Printf("stories=%d tokens=%d lemmas=%d complete=%d missing=%d fa_missing=%d ckb_missing=%d proper=%d errors=%d\n",
		report.Summary.Stories, report.Summary.Tokens, report.Summary.DistinctLemmas, report.Summary.Complete,
		report.Summary.MissingEntries, report.Summary.MissingPersian, report.Summary.MissingSorani,
		report.Summary.ProperNouns, report.Summary.LookupErrors)
}

func extract(path string) (map[string]*tokenStat, int, int, int, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer zr.Close()
	stats := map[string]*tokenStat{}
	stories, lines, tokens := 0, 0, 0
	for _, file := range zr.File {
		if !strings.HasPrefix(file.Name, "items/") || !strings.HasSuffix(file.Name, ".json") {
			continue
		}
		r, err := file.Open()
		if err != nil {
			return nil, 0, 0, 0, err
		}
		data, err := io.ReadAll(io.LimitReader(r, 16<<20))
		r.Close()
		if err != nil {
			return nil, 0, 0, 0, err
		}
		var story storyItem
		if err := json.Unmarshal(data, &story); err != nil {
			return nil, 0, 0, 0, fmt.Errorf("%s: %w", file.Name, err)
		}
		if story.Language != "en" || story.Audience != "teen_adult" {
			continue
		}
		stories++
		for _, cue := range story.Transcript {
			lines++
			indices := wordPattern.FindAllStringIndex(cue.Text, -1)
			for _, index := range indices {
				raw := cue.Text[index[0]:index[1]]
				normalized := normalizeToken(raw)
				if normalized == "" {
					continue
				}
				tokens++
				stat := stats[normalized]
				if stat == nil {
					stat = &tokenStat{Forms: map[string]int{}}
					stats[normalized] = stat
				}
				stat.Count++
				stat.Forms[raw]++
				if firstRuneLower(raw) {
					stat.Lowercase++
				} else if firstRuneUpper(raw) && !sentenceStart(cue.Text, index[0]) && normalized != "i" {
					stat.CapitalMid++
				}
				if len(stat.Contexts) < 3 && !contains(stat.Contexts, cue.Text) {
					stat.Contexts = append(stat.Contexts, cue.Text)
				}
			}
		}
	}
	return stats, stories, lines, tokens, nil
}

func aggregate(surfaces map[string]*tokenStat) []lemmaReport {
	byLemma := map[string]*lemmaReport{}
	formSets := map[string]map[string]bool{}
	for surface, stat := range surfaces {
		proper := stat.CapitalMid > 0 && stat.Lowercase == 0 && stat.CapitalMid*2 >= stat.Count
		lemma := canonicalLemma(surface)
		if proper {
			lemma = surface
		}
		report := byLemma[lemma]
		if report == nil {
			report = &lemmaReport{Lemma: lemma, ProperNoun: proper}
			byLemma[lemma] = report
			formSets[lemma] = map[string]bool{}
		}
		report.Count += stat.Count
		report.ProperNoun = report.ProperNoun && proper
		for form := range stat.Forms {
			formSets[lemma][form] = true
		}
		for _, context := range stat.Contexts {
			if len(report.Contexts) < 3 && !contains(report.Contexts, context) {
				report.Contexts = append(report.Contexts, context)
			}
		}
	}
	out := make([]lemmaReport, 0, len(byLemma))
	for lemma, report := range byLemma {
		for form := range formSets[lemma] {
			report.Forms = append(report.Forms, form)
		}
		sort.Strings(report.Forms)
		out = append(out, *report)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Lemma < out[j].Lemma
	})
	return out
}

func canonicalLemma(surface string) string {
	if lemma := irregular[surface]; lemma != "" {
		return lemma
	}
	if contraction[surface] {
		return surface
	}
	if strings.HasSuffix(surface, "'s") {
		return strings.TrimSuffix(surface, "'s")
	}
	if strings.HasSuffix(surface, "ies") && len(surface) > 4 {
		return strings.TrimSuffix(surface, "ies") + "y"
	}
	if strings.HasSuffix(surface, "ves") && len(surface) > 4 {
		return strings.TrimSuffix(surface, "ves") + "f"
	}
	if strings.HasSuffix(surface, "ied") && len(surface) > 4 {
		return strings.TrimSuffix(surface, "ied") + "y"
	}
	if strings.HasSuffix(surface, "ed") && len(surface) > 4 {
		stem := strings.TrimSuffix(surface, "ed")
		if doubledInflectionConsonant(stem) {
			return stem[:len(stem)-1]
		}
		if hasAnySuffix(stem, "at", "creat", "celebrat", "not", "lov", "mov", "us", "clos", "forc", "giv", "liv", "writ") {
			return stem + "e"
		}
		return stem
	}
	if strings.HasSuffix(surface, "ing") && len(surface) > 5 {
		stem := strings.TrimSuffix(surface, "ing")
		if doubledInflectionConsonant(stem) {
			return stem[:len(stem)-1]
		}
		if hasAnySuffix(stem, "mak", "tak", "com", "hav", "giv", "us", "mov", "clos", "forc", "creat", "writ", "liv") {
			return stem + "e"
		}
		return stem
	}
	if strings.HasSuffix(surface, "es") && len(surface) > 4 {
		withoutES := strings.TrimSuffix(surface, "es")
		if hasAnySuffix(withoutES, "s", "x", "z", "ch", "sh") {
			return withoutES
		}
		return strings.TrimSuffix(surface, "s")
	}
	if strings.HasSuffix(surface, "s") && len(surface) > 3 && !hasAnySuffix(surface, "ss", "us", "is", "news") {
		return strings.TrimSuffix(surface, "s")
	}
	return surface
}

func doubledInflectionConsonant(value string) bool {
	if len(value) < 2 || value[len(value)-1] != value[len(value)-2] {
		return false
	}
	return strings.ContainsRune("bdgmnprt", rune(value[len(value)-1]))
}

func hasAnySuffix(value string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

var irregular = map[string]string{
	"am": "be", "is": "be", "are": "be", "was": "be", "were": "be", "been": "be", "being": "be",
	"has": "have", "had": "have", "having": "have", "does": "do", "did": "do", "done": "do", "doing": "do",
	"said": "say", "says": "say", "went": "go", "gone": "go", "came": "come", "made": "make",
	"took": "take", "taken": "take", "saw": "see", "seen": "see", "knew": "know", "known": "know",
	"thought": "think", "told": "tell", "found": "find", "gave": "give", "given": "give", "got": "get",
	"felt": "feel", "left": "leave", "heard": "hear", "brought": "bring", "began": "begin", "begun": "begin",
	"wrote": "write", "written": "write", "spoke": "speak", "spoken": "speak", "ran": "run", "sat": "sit",
	"stood": "stand", "lay": "lie", "lost": "lose", "kept": "keep", "held": "hold", "became": "become",
	"men": "man", "women": "woman", "children": "child", "people": "person", "feet": "foot", "teeth": "tooth",
	"mice": "mouse", "geese": "goose", "better": "good", "best": "good", "worse": "bad", "worst": "bad",
	"lives": "live", "leaves": "leave", "knives": "knife", "wives": "wife", "wolves": "wolf", "shelves": "shelf", "halves": "half",
}

var contraction = map[string]bool{
	"i'm": true, "i've": true, "i'll": true, "i'd": true,
	"you're": true, "you've": true, "you'll": true, "you'd": true,
	"he's": true, "he'll": true, "he'd": true, "she's": true, "she'll": true, "she'd": true,
	"it's": true, "it'll": true, "we're": true, "we've": true, "we'll": true, "we'd": true,
	"they're": true, "they've": true, "they'll": true, "they'd": true,
	"that's": true, "what's": true, "who's": true, "there's": true, "here's": true, "let's": true,
	"don't": true, "doesn't": true, "didn't": true, "isn't": true, "aren't": true, "wasn't": true, "weren't": true,
	"haven't": true, "hasn't": true, "hadn't": true, "can't": true, "couldn't": true, "won't": true, "wouldn't": true,
	"shouldn't": true, "mustn't": true,
}

func lookupAll(ctx context.Context, api string, lemmas []lemmaReport, concurrency int) {
	base := strings.TrimRight(api, "/")
	if !strings.Contains(base, "/api/") {
		base += "/api/v1"
	}
	client := &http.Client{Timeout: 15 * time.Second}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if lemmas[index].ProperNoun {
					lemmas[index].Status = "proper_noun"
					continue
				}
				lookupOne(ctx, client, base, &lemmas[index])
			}
		}()
	}
	for index := range lemmas {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
}

func lookupOne(ctx context.Context, client *http.Client, base string, report *lemmaReport) {
	endpoint := base + "/vocab/lookup?lang=en&lemma=" + url.QueryEscape(report.Lemma)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("User-Agent", "LeksuuStoryVocabularyAudit/1.0")
	res, err := client.Do(req)
	if err != nil {
		report.Status, report.LookupError = "lookup_error", err.Error()
		return
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		report.Status = "missing_entry"
		return
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		report.Status, report.LookupError = "lookup_error", fmt.Sprintf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
		return
	}
	var payload struct {
		Lemma         string            `json:"lemma"`
		Meaning       map[string]string `json:"meaning"`
		Pronunciation string            `json:"pronunciation"`
		Example       string            `json:"example"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		report.Status, report.LookupError = "lookup_error", err.Error()
		return
	}
	report.DatabaseLemma, report.Meaning, report.Pronunciation, report.Example = payload.Lemma, payload.Meaning, payload.Pronunciation, payload.Example
	hasFA := strings.TrimSpace(payload.Meaning["fa"]) != ""
	hasCKB := strings.TrimSpace(payload.Meaning["ckb"]) != ""
	switch {
	case hasFA && hasCKB:
		report.Status = "complete"
	case !hasFA && !hasCKB:
		report.Status = "missing_fa_ckb"
	case !hasFA:
		report.Status = "missing_fa"
	default:
		report.Status = "missing_ckb"
	}
}

func summarize(generated, archive, api string, stories, lines, tokens, surfaces int, lemmas []lemmaReport) summary {
	s := summary{GeneratedAt: generated, Archive: archive, API: api, Stories: stories, TranscriptLines: lines, Tokens: tokens, DistinctSurfaces: surfaces, DistinctLemmas: len(lemmas)}
	for _, lemma := range lemmas {
		if lemma.ProperNoun {
			s.ProperNouns++
			continue
		}
		s.CoverageDenominator++
		switch lemma.Status {
		case "complete":
			s.Complete++
		case "missing_entry":
			s.MissingEntries++
		case "missing_fa":
			s.MissingPersian++
		case "missing_ckb":
			s.MissingSorani++
		case "missing_fa_ckb":
			s.MissingPersian++
			s.MissingSorani++
		case "lookup_error":
			s.LookupErrors++
		}
	}
	if s.CoverageDenominator > 0 {
		s.CompletePercent = float64(s.Complete) * 100 / float64(s.CoverageDenominator)
	}
	return s
}

func buildTemplate(generated string, lemmas []lemmaReport) importTemplate {
	template := importTemplate{Schema: "leksuu.vocabulary-review.v1", GeneratedAt: generated, Notice: "Fill only the listed needs from validated sources. Set verified=true only after bilingual review. The importer never overwrites existing nonblank meanings."}
	for _, lemma := range lemmas {
		if lemma.ProperNoun || lemma.Status == "complete" || lemma.Status == "lookup_error" {
			continue
		}
		needs := []string{}
		meaning := map[string]string{}
		if lemma.Status == "missing_entry" || strings.Contains(lemma.Status, "fa") {
			needs = append(needs, "fa")
			meaning["fa"] = ""
		}
		if lemma.Status == "missing_entry" || strings.Contains(lemma.Status, "ckb") {
			needs = append(needs, "ckb")
			meaning["ckb"] = ""
		}
		example := ""
		if len(lemma.Contexts) > 0 {
			example = lemma.Contexts[0]
		}
		template.Items = append(template.Items, importTemplateRow{Language: "en", Kind: "word", Lemma: lemma.Lemma, Meaning: meaning, ExistingMeaning: lemma.Meaning, Example: example, Source: "teen-adult-stories-reviewed", Needs: needs, Verified: false})
	}
	return template
}

func markdown(report coverageReport) string {
	s := report.Summary
	var b strings.Builder
	fmt.Fprintf(&b, "# Teen & Adult story vocabulary coverage\n\nGenerated: `%s`\n\n", s.GeneratedAt)
	fmt.Fprintf(&b, "- Corpus: %d stories, %d transcript lines, %d word tokens\n", s.Stories, s.TranscriptLines, s.Tokens)
	fmt.Fprintf(&b, "- Normalized vocabulary: %d surface forms → %d lemmas\n", s.DistinctSurfaces, s.DistinctLemmas)
	fmt.Fprintf(&b, "- Proper nouns excluded from definition coverage: %d\n", s.ProperNouns)
	fmt.Fprintf(&b, "- Complete Persian + Sorani entries: %d/%d (**%.2f%%**)\n", s.Complete, s.CoverageDenominator, s.CompletePercent)
	fmt.Fprintf(&b, "- Missing dictionary entries: %d\n- Existing entries missing Persian: %d\n- Existing entries missing Sorani: %d\n- Lookup errors (not counted as missing): %d\n\n", s.MissingEntries, s.MissingPersian, s.MissingSorani, s.LookupErrors)
	b.WriteString("Coverage is type-based (distinct lemmas), not token-weighted. A proper noun is listed separately and is not treated as a missing definition. Lemmatization is deterministic and reviewable; the JSON report retains surface forms and story contexts.\n\n")
	writeSection := func(title string, match func(lemmaReport) bool, limit int) {
		fmt.Fprintf(&b, "## %s\n\n| Lemma | Uses | Forms |\n|---|---:|---|\n", title)
		n := 0
		for _, lemma := range report.Lemmas {
			if !match(lemma) {
				continue
			}
			fmt.Fprintf(&b, "| %s | %d | %s |\n", escapeCell(lemma.Lemma), lemma.Count, escapeCell(strings.Join(lemma.Forms, ", ")))
			n++
			if limit > 0 && n >= limit {
				break
			}
		}
		if n == 0 {
			b.WriteString("| _None_ |  |  |\n")
		}
		b.WriteString("\n")
	}
	writeSection("Highest-frequency missing entries", func(l lemmaReport) bool { return l.Status == "missing_entry" }, 200)
	writeSection("Entries missing Persian", func(l lemmaReport) bool { return l.Status == "missing_fa" || l.Status == "missing_fa_ckb" }, 0)
	writeSection("Entries missing Sorani", func(l lemmaReport) bool { return l.Status == "missing_ckb" || l.Status == "missing_fa_ckb" }, 0)
	writeSection("Proper nouns", func(l lemmaReport) bool { return l.ProperNoun }, 0)
	return b.String()
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "’", "'"))
}

func sentenceStart(text string, offset int) bool {
	for offset > 0 {
		offset--
		r := rune(text[offset])
		if unicode.IsSpace(r) || strings.ContainsRune("\"'“”‘’()[]", r) {
			continue
		}
		return strings.ContainsRune(".!?", r)
	}
	return true
}

func firstRuneUpper(value string) bool {
	for _, r := range value {
		return unicode.IsUpper(r)
	}
	return false
}

func firstRuneLower(value string) bool {
	for _, r := range value {
		return unicode.IsLower(r)
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func escapeCell(value string) string { return strings.ReplaceAll(value, "|", "\\|") }

func writeJSON(path string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatalf("encode %s: %v", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatalf("write %s: %v", path, err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
