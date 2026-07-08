// Package repo holds SRS repositories. memory.go is the default (stdlib only);
// postgres.go (build tag pgx) maps onto vocabulary_items / srs_cards.
package repo

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/shared/id"
)

type MemorySRSRepo struct {
	mu          sync.Mutex
	vocabByKey  map[string]*domain.Vocab // language|kind|lemma -> vocab
	vocabByID   map[string]*domain.Vocab
	cards       map[string]*domain.Card // cardID -> card
	cardByOwner map[string]string       // userID|vocabID -> cardID
	reviewLog   map[string][]time.Time  // userID -> review timestamps
	settings    map[string]int          // userID -> daily goal
}

func NewMemorySRSRepo() *MemorySRSRepo {
	r := &MemorySRSRepo{
		vocabByKey:  map[string]*domain.Vocab{},
		vocabByID:   map[string]*domain.Vocab{},
		cards:       map[string]*domain.Card{},
		cardByOwner: map[string]string{},
		reviewLog:   map[string][]time.Time{},
		settings:    map[string]int{},
	}
	r.seedVocab()
	return r
}

func (r *MemorySRSRepo) seedVocab() {
	ctx := context.Background()
	vocab := []domain.Vocab{
		{Language: "en", Kind: "word", Lemma: "hello", Pronunciation: "/h\u0259\u02c8lo\u028a/", Meaning: map[string]string{"fa": "سلام", "ckb": "سڵاو"}, Example: "Hello, how are you?"},
		{Language: "en", Kind: "word", Lemma: "name", Pronunciation: "/ne\u026am/", Meaning: map[string]string{"fa": "نام", "ckb": "ناو"}, Example: "My name is Sara."},
		{Language: "en", Kind: "word", Lemma: "mother", Pronunciation: "/\u02c8m\u028c\u00f0\u0259r/", Meaning: map[string]string{"fa": "مادر", "ckb": "دایک"}, Example: "This is my mother."},
		{Language: "en", Kind: "word", Lemma: "father", Pronunciation: "/\u02c8f\u0251\u02d0\u00f0\u0259r/", Meaning: map[string]string{"fa": "پدر", "ckb": "باوک"}, Example: "He is my father."},
		{Language: "en", Kind: "word", Lemma: "brother", Pronunciation: "/\u02c8br\u028c\u00f0\u0259r/", Meaning: map[string]string{"fa": "برادر", "ckb": "برا"}, Example: "He is my brother."},
		{Language: "en", Kind: "word", Lemma: "sister", Pronunciation: "/\u02c8s\u026ast\u0259r/", Meaning: map[string]string{"fa": "خواهر", "ckb": "خوشک"}, Example: "She is my sister."},
		{Language: "en", Kind: "word", Lemma: "family", Pronunciation: "/\u02c8f\u00e6m\u0259li/", Meaning: map[string]string{"fa": "خانواده", "ckb": "خێزان"}, Example: "I have a small family."},
		{Language: "en", Kind: "word", Lemma: "one", Pronunciation: "/w\u028cn/", Meaning: map[string]string{"fa": "یک", "ckb": "یەک"}, Example: "I have one book."},
		{Language: "en", Kind: "word", Lemma: "two", Pronunciation: "/tu\u02d0/", Meaning: map[string]string{"fa": "دو", "ckb": "دوو"}, Example: "I have two books."},
		{Language: "en", Kind: "word", Lemma: "three", Pronunciation: "/\u03b8ri\u02d0/", Meaning: map[string]string{"fa": "سه", "ckb": "سێ"}, Example: "Three apples."},
		{Language: "en", Kind: "word", Lemma: "four", Pronunciation: "/f\u0254\u02d0r/", Meaning: map[string]string{"fa": "چهار", "ckb": "چوار"}, Example: "Four cats."},
		{Language: "en", Kind: "word", Lemma: "five", Pronunciation: "/fa\u026av/", Meaning: map[string]string{"fa": "پنج", "ckb": "پێنج"}, Example: "It is five dollars."},
		{Language: "en", Kind: "word", Lemma: "book", Pronunciation: "/b\u028ak/", Meaning: map[string]string{"fa": "کتاب", "ckb": "پەرتووک"}, Example: "I read a book."},
		{Language: "en", Kind: "word", Lemma: "apple", Pronunciation: "/\u02c8\u00e6p\u0259l/", Meaning: map[string]string{"fa": "سیب", "ckb": "سێو"}, Example: "The apple is red."},
		{Language: "en", Kind: "word", Lemma: "coffee", Pronunciation: "/\u02c8k\u0254\u02d0fi/", Meaning: map[string]string{"fa": "قهوه", "ckb": "قاوە"}, Example: "I like coffee."},
		{Language: "en", Kind: "word", Lemma: "water", Pronunciation: "/\u02c8w\u0254\u02d0t\u0259r/", Meaning: map[string]string{"fa": "آب", "ckb": "ئاو"}, Example: "She wants water."},
		{Language: "en", Kind: "word", Lemma: "tea", Pronunciation: "/ti\u02d0/", Meaning: map[string]string{"fa": "چای", "ckb": "چا"}, Example: "I like tea."},
		{Language: "en", Kind: "word", Lemma: "bread", Pronunciation: "/bred/", Meaning: map[string]string{"fa": "نان", "ckb": "نان"}, Example: "She buys bread."},
		{Language: "en", Kind: "word", Lemma: "milk", Pronunciation: "/m\u026alk/", Meaning: map[string]string{"fa": "شیر", "ckb": "شیر"}, Example: "She buys milk."},
		{Language: "en", Kind: "word", Lemma: "red", Pronunciation: "/red/", Meaning: map[string]string{"fa": "قرمز", "ckb": "سوور"}, Example: "The apple is red."},
		{Language: "en", Kind: "word", Lemma: "blue", Pronunciation: "/blu\u02d0/", Meaning: map[string]string{"fa": "آبی", "ckb": "شین"}, Example: "The sky is blue."},
		{Language: "en", Kind: "word", Lemma: "green", Pronunciation: "/\u0261ri\u02d0n/", Meaning: map[string]string{"fa": "سبز", "ckb": "سەوز"}, Example: "Grass is green."},
		{Language: "en", Kind: "word", Lemma: "big", Pronunciation: "/b\u026a\u0261/", Meaning: map[string]string{"fa": "بزرگ", "ckb": "گەورە"}, Example: "The elephant is big."},
		{Language: "en", Kind: "word", Lemma: "small", Pronunciation: "/sm\u0254\u02d0l/", Meaning: map[string]string{"fa": "کوچک", "ckb": "بچووک"}, Example: "The mouse is small."},
		{Language: "en", Kind: "word", Lemma: "time", Pronunciation: "/ta\u026am/", Meaning: map[string]string{"fa": "زمان", "ckb": "کات"}, Example: "What time is it?"},
		{Language: "en", Kind: "word", Lemma: "hour", Pronunciation: "/\u02c8a\u028a\u0259r/", Meaning: map[string]string{"fa": "ساعت", "ckb": "کاتژمێر"}, Example: "One hour."},
		{Language: "en", Kind: "word", Lemma: "money", Pronunciation: "/\u02c8m\u028cni/", Meaning: map[string]string{"fa": "پول", "ckb": "پارە"}, Example: "I have money."},
		{Language: "en", Kind: "word", Lemma: "shop", Pronunciation: "/\u0283\u0252p/", Meaning: map[string]string{"fa": "مغازه", "ckb": "دوکان"}, Example: "She goes to the shop."},
		{Language: "en", Kind: "word", Lemma: "left", Pronunciation: "/left/", Meaning: map[string]string{"fa": "چپ", "ckb": "چەپ"}, Example: "Turn left."},
		{Language: "en", Kind: "word", Lemma: "right", Pronunciation: "/ra\u026at/", Meaning: map[string]string{"fa": "راست", "ckb": "ڕاست"}, Example: "The shop is on the right."},
		{Language: "en", Kind: "word", Lemma: "work", Pronunciation: "/w\u025c\u02d0rk/", Meaning: map[string]string{"fa": "کار", "ckb": "کار"}, Example: "She goes to work."},
		{Language: "en", Kind: "word", Lemma: "like", Pronunciation: "/la\u026ak/", Meaning: map[string]string{"fa": "دوست داشتن", "ckb": "حەزکردن"}, Example: "I like coffee."},
		{Language: "en", Kind: "word", Lemma: "monday", Pronunciation: "/\u02c8m\u028cnde\u026a/", Meaning: map[string]string{"fa": "دوشنبه", "ckb": "دووشەممە"}, Example: "Today is Monday."},
		{Language: "en", Kind: "word", Lemma: "today", Pronunciation: "/t\u0259\u02c8de\u026a/", Meaning: map[string]string{"fa": "امروز", "ckb": "ئەمڕۆ"}, Example: "It is hot today."},
		{Language: "en", Kind: "word", Lemma: "tomorrow", Pronunciation: "/t\u0259\u02c8m\u0252ro\u028a/", Meaning: map[string]string{"fa": "فردا", "ckb": "سبەینێ"}, Example: "Tomorrow is Tuesday."},
		{Language: "en", Kind: "word", Lemma: "week", Pronunciation: "/wi\u02d0k/", Meaning: map[string]string{"fa": "هفته", "ckb": "هەفتە"}, Example: "There are seven days in a week."},
		{Language: "en", Kind: "word", Lemma: "day", Pronunciation: "/de\u026a/", Meaning: map[string]string{"fa": "روز", "ckb": "ڕۆژ"}, Example: "It is a sunny day."},
		{Language: "en", Kind: "word", Lemma: "hot", Pronunciation: "/h\u0252t/", Meaning: map[string]string{"fa": "گرم", "ckb": "گەرم"}, Example: "It is hot today."},
		{Language: "en", Kind: "word", Lemma: "cold", Pronunciation: "/ko\u028ald/", Meaning: map[string]string{"fa": "سرد", "ckb": "سارد"}, Example: "It is cold in winter."},
		{Language: "en", Kind: "word", Lemma: "rain", Pronunciation: "/re\u026an/", Meaning: map[string]string{"fa": "باران", "ckb": "باران"}, Example: "I like the rain."},
		{Language: "en", Kind: "word", Lemma: "sun", Pronunciation: "/s\u028cn/", Meaning: map[string]string{"fa": "خورشید", "ckb": "خۆر"}, Example: "The sun is hot."},
		{Language: "en", Kind: "word", Lemma: "snow", Pronunciation: "/sno\u028a/", Meaning: map[string]string{"fa": "برف", "ckb": "بەفر"}, Example: "Snow is white."},
		{Language: "en", Kind: "word", Lemma: "umbrella", Pronunciation: "/\u028cm\u02c8brel\u0259/", Meaning: map[string]string{"fa": "چتر", "ckb": "چەتر"}, Example: "She takes her umbrella."},
		{Language: "en", Kind: "word", Lemma: "winter", Pronunciation: "/\u02c8w\u026ant\u0259r/", Meaning: map[string]string{"fa": "زمستان", "ckb": "زستان"}, Example: "It is cold in winter."},
	}
	for i := range vocab {
		_ = r.EnsureVocab(ctx, &vocab[i])
	}
}

func vkey(lang, kind, lemma string) string { return lang + "|" + kind + "|" + lemma }

func (r *MemorySRSRepo) EnsureVocab(_ context.Context, v *domain.Vocab) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := vkey(v.Language, v.Kind, v.Lemma)
	if existing, ok := r.vocabByKey[k]; ok {
		if len(v.Meaning) > 0 {
			existing.Meaning = v.Meaning
		}
		if v.Example != "" {
			existing.Example = v.Example
		}
		if v.Pronunciation != "" {
			existing.Pronunciation = v.Pronunciation
		}
		if v.AudioKey != "" {
			existing.AudioKey = v.AudioKey
		}
		v.ID = existing.ID
		return nil
	}
	v.ID = id.New()
	cp := *v
	r.vocabByKey[k] = &cp
	r.vocabByID[v.ID] = &cp
	return nil
}

func (r *MemorySRSRepo) EnsureCard(_ context.Context, c *domain.Card) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	owner := c.UserID + "|" + c.VocabID
	if cid, ok := r.cardByOwner[owner]; ok {
		*c = *r.cards[cid] // return the existing card's state
		return false, nil
	}
	c.ID = id.New()
	cp := *c
	r.cards[c.ID] = &cp
	r.cardByOwner[owner] = c.ID
	return true, nil
}

func (r *MemorySRSRepo) Card(_ context.Context, userID, cardID string) (*domain.Card, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cards[cardID]
	if !ok || c.UserID != userID {
		return nil, domain.ErrCardNotFound
	}
	cp := *c
	return &cp, nil
}

func (r *MemorySRSRepo) UpdateCard(_ context.Context, c *domain.Card) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.cards[c.ID]
	if !ok || existing.UserID != c.UserID {
		return domain.ErrCardNotFound
	}
	cp := *c
	r.cards[c.ID] = &cp
	return nil
}

func (r *MemorySRSRepo) Due(_ context.Context, userID, language string, now time.Time, limit int) ([]domain.DueCard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.DueCard
	for _, c := range r.cards {
		if c.UserID != userID || c.Suspended || c.DueAt.After(now) {
			continue
		}
		v := r.vocabByID[c.VocabID]
		if v == nil || (language != "" && v.Language != language) {
			continue
		}
		out = append(out, domain.DueCard{
			CardID: c.ID, Box: c.Box, DueAt: c.DueAt,
			Language: v.Language, Lemma: v.Lemma, Meaning: v.Meaning, Example: v.Example, Clip: c.Clip,
			IntervalDays: c.IntervalDays, Reps: c.Reps, Lapses: c.Lapses, Ease: c.Ease,
		})
	}
	// Oldest-due first (no starvation); among the same due moment, weaker cards
	// (lower ease, more lapses) come first so struggling items get attention.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.DueAt.Equal(b.DueAt) {
			return a.DueAt.Before(b.DueAt)
		}
		if a.Ease != b.Ease {
			return a.Ease < b.Ease
		}
		return a.Lapses > b.Lapses
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemorySRSRepo) BoxCounts(_ context.Context, userID string) (map[int]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[int]int{}
	for _, c := range r.cards {
		if c.UserID == userID {
			out[c.Box]++
		}
	}
	return out, nil
}

func (r *MemorySRSRepo) DueCount(_ context.Context, userID string, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, c := range r.cards {
		if c.UserID == userID && !c.DueAt.After(now) {
			n++
		}
	}
	return n, nil
}

func (r *MemorySRSRepo) Forecast(_ context.Context, userID string, now time.Time, days int) ([]domain.DayForecast, error) {
	if days <= 0 {
		days = 14
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	nowU := now.UTC()
	today := time.Date(nowU.Year(), nowU.Month(), nowU.Day(), 0, 0, 0, 0, time.UTC)
	counts := make([]int, days)
	for _, c := range r.cards {
		if c.UserID != userID {
			continue
		}
		du := c.DueAt.UTC()
		day := time.Date(du.Year(), du.Month(), du.Day(), 0, 0, 0, 0, time.UTC)
		off := int(day.Sub(today).Hours()) / 24
		if off < 0 {
			off = 0 // overdue folds into today
		}
		if off >= days {
			continue
		}
		counts[off]++
	}
	out := make([]domain.DayForecast, days)
	for i := 0; i < days; i++ {
		out[i] = domain.DayForecast{Offset: i, Date: today.AddDate(0, 0, i).Format("2006-01-02"), Due: counts[i]}
	}
	return out, nil
}

func (r *MemorySRSRepo) Leeches(_ context.Context, userID string, minLapses, limit int) ([]domain.LeechCard, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.LeechCard
	for _, c := range r.cards {
		if c.UserID != userID || c.Lapses < minLapses {
			continue
		}
		v := r.vocabByID[c.VocabID]
		if v == nil {
			continue
		}
		out = append(out, domain.LeechCard{
			CardID: c.ID, Lemma: v.Lemma, Meaning: v.Meaning, Lapses: c.Lapses, Box: c.Box, Suspended: c.Suspended, Source: c.Source,
		})
	}
	// Hardest first (most lapses), then least mature.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Lapses != out[j].Lapses {
			return out[i].Lapses > out[j].Lapses
		}
		return out[i].Box < out[j].Box
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *MemorySRSRepo) LogReview(_ context.Context, userID string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reviewLog[userID] = append(r.reviewLog[userID], at.UTC())
	return nil
}

func (r *MemorySRSRepo) ReviewActivity(_ context.Context, userID string, now time.Time) (today, streak int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	nowU := now.UTC()
	todayStart := time.Date(nowU.Year(), nowU.Month(), nowU.Day(), 0, 0, 0, 0, time.UTC)
	days := map[string]bool{} // distinct UTC review dates
	for _, t := range r.reviewLog[userID] {
		tu := t.UTC()
		d := time.Date(tu.Year(), tu.Month(), tu.Day(), 0, 0, 0, 0, time.UTC)
		days[d.Format("2006-01-02")] = true
		if !tu.Before(todayStart) {
			today++
		}
	}
	streak = streakFromDays(days, todayStart)
	return today, streak, nil
}

func (r *MemorySRSRepo) ReviewWeek(_ context.Context, userID string, now time.Time) (map[string]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	nowU := now.UTC()
	today := time.Date(nowU.Year(), nowU.Month(), nowU.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, -6) // include the last 7 days (today .. today-6)
	out := map[string]int{}
	for _, t := range r.reviewLog[userID] {
		tu := t.UTC()
		d := time.Date(tu.Year(), tu.Month(), tu.Day(), 0, 0, 0, 0, time.UTC)
		if d.Before(cutoff) {
			continue
		}
		out[d.Format("2006-01-02")]++
	}
	return out, nil
}

// streakFromDays counts consecutive days with activity ending at today (or
// yesterday, so a not-yet-reviewed today doesn't break a live streak).
func streakFromDays(days map[string]bool, todayStart time.Time) int {
	if len(days) == 0 {
		return 0
	}
	start := todayStart
	if !days[todayStart.Format("2006-01-02")] {
		start = todayStart.AddDate(0, 0, -1) // grace: count a streak ending yesterday
	}
	streak := 0
	for d := start; days[d.Format("2006-01-02")]; d = d.AddDate(0, 0, -1) {
		streak++
	}
	return streak
}

func (r *MemorySRSRepo) ListVocab(_ context.Context, language, q string, limit, offset int) ([]domain.Vocab, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q = strings.ToLower(strings.TrimSpace(q))
	var all []domain.Vocab
	for _, v := range r.vocabByID {
		if language != "" && v.Language != language {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Lemma), q) {
			continue
		}
		all = append(all, *v)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Lemma < all[j].Lemma })
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return append([]domain.Vocab(nil), all[offset:end]...), total, nil
}

func (r *MemorySRSRepo) GetVocab(_ context.Context, vid string) (*domain.Vocab, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.vocabByID[vid]; ok {
		cp := *v
		return &cp, nil
	}
	return nil, domain.ErrVocabNotFound
}

func (r *MemorySRSRepo) UpdateVocab(_ context.Context, v *domain.Vocab) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.vocabByID[v.ID]
	if !ok {
		return domain.ErrVocabNotFound
	}
	existing.Meaning = v.Meaning
	existing.Example = v.Example
	existing.Pronunciation = v.Pronunciation
	existing.AudioKey = v.AudioKey
	return nil
}

func (r *MemorySRSRepo) DeleteVocab(_ context.Context, vid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.vocabByID[vid]
	if !ok {
		return domain.ErrVocabNotFound
	}
	delete(r.vocabByID, vid)
	delete(r.vocabByKey, vkey(v.Language, v.Kind, v.Lemma))
	return nil
}

func (r *MemorySRSRepo) LookupVocab(_ context.Context, language, lemma string) (*domain.Vocab, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.vocabByKey[vkey(language, "word", strings.ToLower(strings.TrimSpace(lemma)))]; ok {
		cp := *v
		return &cp, nil
	}
	return nil, domain.ErrVocabNotFound
}

func (r *MemorySRSRepo) GetGoal(_ context.Context, userID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.settings[userID]; ok {
		return g, nil
	}
	return domain.DailyReviewGoal, nil
}

func (r *MemorySRSRepo) SetGoal(_ context.Context, userID string, goal int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settings[userID] = goal
	return nil
}

func (r *MemorySRSRepo) MyCards(_ context.Context, userID, query, status string, limit, offset int) ([]domain.CardRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := []domain.CardRow{}
	q := strings.ToLower(strings.TrimSpace(query))
	for _, c := range r.cards {
		if c.UserID != userID {
			continue
		}
		v := r.vocabByID[c.VocabID]
		if v == nil {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(v.Lemma), q) {
			continue
		}
		leech := c.Lapses >= domain.LeechLapses
		switch status {
		case "suspended":
			if !c.Suspended {
				continue
			}
		case "active":
			if c.Suspended {
				continue
			}
		case "leech":
			if !leech {
				continue
			}
		}
		rows = append(rows, domain.CardRow{
			CardID: c.ID, Lemma: v.Lemma, Language: v.Language, Meaning: v.Meaning,
			Box: c.Box, IntervalDays: c.IntervalDays, Lapses: c.Lapses,
			Suspended: c.Suspended, Leech: leech, DueAt: c.DueAt,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].DueAt.Before(rows[j].DueAt) })
	if offset >= len(rows) {
		return []domain.CardRow{}, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end], nil
}

func (r *MemorySRSRepo) DeleteCard(_ context.Context, userID, cardID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.cards[cardID]
	if !ok || c.UserID != userID {
		return domain.ErrCardNotFound
	}
	delete(r.cards, cardID)
	return nil
}
