//go:build pgx

package repo

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leksuu/platform/internal/modules/srs/domain"
	"strconv"
	"strings"
)

type PostgresSRSRepo struct{ pool *pgxpool.Pool }

func NewPostgresSRSRepo(pool *pgxpool.Pool) *PostgresSRSRepo { return &PostgresSRSRepo{pool: pool} }

func (r *PostgresSRSRepo) EnsureVocab(ctx context.Context, v *domain.Vocab) error {
	meaning, _ := json.Marshal(v.Meaning)
	return r.pool.QueryRow(ctx,
		`INSERT INTO vocabulary_items (language, item_kind, lemma, pronunciation, meaning, example, audio_key)
		 VALUES ($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''),NULLIF($7,''))
		 ON CONFLICT (language, item_kind, lemma)
		 DO UPDATE SET meaning = EXCLUDED.meaning,
		   example = COALESCE(EXCLUDED.example, vocabulary_items.example),
		   pronunciation = COALESCE(EXCLUDED.pronunciation, vocabulary_items.pronunciation),
		   audio_key = COALESCE(EXCLUDED.audio_key, vocabulary_items.audio_key)
		 RETURNING id`,
		v.Language, v.Kind, v.Lemma, v.Pronunciation, meaning, v.Example, v.AudioKey).Scan(&v.ID)
}

func (r *PostgresSRSRepo) EnsureCard(ctx context.Context, c *domain.Card) (bool, error) {
	var clip []byte
	if c.Clip != nil {
		clip, _ = json.Marshal(c.Clip)
	}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO srs_cards (user_id, vocab_id, box, interval_days, ease, due_at, source, clip)
		 VALUES ($1,$2,1,0,$3,$4,$5,$6)
		 ON CONFLICT (user_id, vocab_id) DO NOTHING
		 RETURNING id`,
		c.UserID, c.VocabID, easeOrDefault(c.Ease), c.DueAt, c.Source, clip).Scan(&c.ID)
	if err == nil {
		c.Box = 1
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		e := r.pool.QueryRow(ctx,
			`SELECT id, box, interval_days, ease, reps, lapses, due_at FROM srs_cards WHERE user_id=$1 AND vocab_id=$2`,
			c.UserID, c.VocabID).Scan(&c.ID, &c.Box, &c.IntervalDays, &c.Ease, &c.Reps, &c.Lapses, &c.DueAt)
		return false, e
	}
	return false, err
}

func (r *PostgresSRSRepo) Card(ctx context.Context, userID, cardID string) (*domain.Card, error) {
	var c domain.Card
	c.UserID = userID
	var prev []byte
	err := r.pool.QueryRow(ctx,
		`SELECT id, vocab_id, box, interval_days, ease, reps, lapses, suspended, due_at, COALESCE(source,''), prev_state
		 FROM srs_cards WHERE id=$1 AND user_id=$2`,
		cardID, userID).Scan(&c.ID, &c.VocabID, &c.Box, &c.IntervalDays, &c.Ease, &c.Reps, &c.Lapses, &c.Suspended, &c.DueAt, &c.Source, &prev)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrCardNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(prev) > 0 {
		var snap domain.CardSnapshot
		if json.Unmarshal(prev, &snap) == nil {
			c.Prev = &snap
		}
	}
	return &c, nil
}

func (r *PostgresSRSRepo) UpdateCard(ctx context.Context, c *domain.Card) error {
	var prev []byte
	if c.Prev != nil {
		prev, _ = json.Marshal(c.Prev)
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE srs_cards SET box=$1, interval_days=$2, ease=$3, reps=$4, lapses=$5, suspended=$6, due_at=$7, prev_state=$8, updated_at=now()
		 WHERE id=$9 AND user_id=$10`,
		c.Box, c.IntervalDays, easeOrDefault(c.Ease), c.Reps, c.Lapses, c.Suspended, c.DueAt, prev, c.ID, c.UserID)
	return err
}

func (r *PostgresSRSRepo) Due(ctx context.Context, userID, language string, now time.Time, limit int) ([]domain.DueCard, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.box, c.due_at, v.language, v.lemma, v.meaning, COALESCE(v.example,''), c.clip, c.interval_days, c.reps, c.lapses, c.ease
		 FROM srs_cards c JOIN vocabulary_items v ON v.id = c.vocab_id
		 WHERE c.user_id=$1 AND c.suspended = false AND ($2 = '' OR v.language = $2) AND c.due_at <= $3
		 ORDER BY c.due_at ASC, c.ease ASC, c.lapses DESC LIMIT $4`,
		userID, language, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DueCard
	for rows.Next() {
		var d domain.DueCard
		var meaning, clip []byte
		if err := rows.Scan(&d.CardID, &d.Box, &d.DueAt, &d.Language, &d.Lemma, &meaning, &d.Example, &clip, &d.IntervalDays, &d.Reps, &d.Lapses, &d.Ease); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(meaning, &d.Meaning)
		if len(clip) > 0 {
			_ = json.Unmarshal(clip, &d.Clip)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *PostgresSRSRepo) BoxCounts(ctx context.Context, userID string) (map[int]int, error) {
	rows, err := r.pool.Query(ctx, `SELECT box, count(*) FROM srs_cards WHERE user_id=$1 GROUP BY box`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var box, n int
		if err := rows.Scan(&box, &n); err != nil {
			return nil, err
		}
		out[box] = n
	}
	return out, rows.Err()
}

func (r *PostgresSRSRepo) DueCount(ctx context.Context, userID string, now time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM srs_cards WHERE user_id=$1 AND due_at <= $2`, userID, now).Scan(&n)
	return n, err
}

func (r *PostgresSRSRepo) Forecast(ctx context.Context, userID string, now time.Time, days int) ([]domain.DayForecast, error) {
	if days <= 0 {
		days = 14
	}
	// Bucket cards by UTC day offset from today; overdue (negative) folds into 0.
	// Days with no cards are filled in below so the series is contiguous.
	rows, err := r.pool.Query(ctx,
		`SELECT greatest(0, ((due_at AT TIME ZONE 'UTC')::date - ($2 AT TIME ZONE 'UTC')::date)) AS off, count(*)
		 FROM srs_cards
		 WHERE user_id=$1 AND (due_at AT TIME ZONE 'UTC')::date < (($2 AT TIME ZONE 'UTC')::date + $3::int)
		 GROUP BY off`,
		userID, now, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int]int, days)
	for rows.Next() {
		var off, n int
		if err := rows.Scan(&off, &n); err != nil {
			return nil, err
		}
		counts[off] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	nowU := now.UTC()
	today := time.Date(nowU.Year(), nowU.Month(), nowU.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]domain.DayForecast, days)
	for i := 0; i < days; i++ {
		out[i] = domain.DayForecast{Offset: i, Date: today.AddDate(0, 0, i).Format("2006-01-02"), Due: counts[i]}
	}
	return out, nil
}

func (r *PostgresSRSRepo) Leeches(ctx context.Context, userID string, minLapses, limit int) ([]domain.LeechCard, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, v.lemma, v.meaning, c.lapses, c.box, c.suspended, COALESCE(c.source,'')
		 FROM srs_cards c JOIN vocabulary_items v ON v.id = c.vocab_id
		 WHERE c.user_id=$1 AND c.lapses >= $2
		 ORDER BY c.lapses DESC, c.box ASC LIMIT $3`,
		userID, minLapses, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LeechCard{}
	for rows.Next() {
		var lc domain.LeechCard
		var meaning []byte
		if err := rows.Scan(&lc.CardID, &lc.Lemma, &meaning, &lc.Lapses, &lc.Box, &lc.Suspended, &lc.Source); err != nil {
			return nil, err
		}
		if len(meaning) > 0 {
			_ = json.Unmarshal(meaning, &lc.Meaning)
		}
		out = append(out, lc)
	}
	return out, rows.Err()
}

func scanVocab(row interface {
	Scan(dest ...any) error
}) (*domain.Vocab, error) {
	var v domain.Vocab
	var meaning []byte
	var pron, example, audio *string
	if err := row.Scan(&v.ID, &v.Language, &v.Kind, &v.Lemma, &pron, &meaning, &example, &audio); err != nil {
		return nil, err
	}
	if pron != nil {
		v.Pronunciation = *pron
	}
	if example != nil {
		v.Example = *example
	}
	if audio != nil {
		v.AudioKey = *audio
	}
	if len(meaning) > 0 {
		_ = json.Unmarshal(meaning, &v.Meaning)
	}
	return &v, nil
}

const vocabCols = `id, language, item_kind, lemma, pronunciation, meaning, example, audio_key`

func (r *PostgresSRSRepo) ListVocab(ctx context.Context, language, q string, limit, offset int) ([]domain.Vocab, int, error) {
	if limit <= 0 {
		limit = 1000
	}
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM vocabulary_items WHERE ($1='' OR language=$1) AND ($2='' OR lemma ILIKE '%'||$2||'%')`,
		language, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+vocabCols+` FROM vocabulary_items
		   WHERE ($1='' OR language=$1) AND ($2='' OR lemma ILIKE '%'||$2||'%')
		   ORDER BY lemma LIMIT $3 OFFSET $4`, language, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Vocab
	for rows.Next() {
		v, err := scanVocab(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *v)
	}
	return out, total, rows.Err()
}

func (r *PostgresSRSRepo) GetVocab(ctx context.Context, vid string) (*domain.Vocab, error) {
	v, err := scanVocab(r.pool.QueryRow(ctx, `SELECT `+vocabCols+` FROM vocabulary_items WHERE id=$1`, vid))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrVocabNotFound
	}
	return v, err
}

func (r *PostgresSRSRepo) UpdateVocab(ctx context.Context, v *domain.Vocab) error {
	meaning, _ := json.Marshal(v.Meaning)
	var id string
	err := r.pool.QueryRow(ctx,
		`UPDATE vocabulary_items SET meaning=$2, example=NULLIF($3,''), pronunciation=NULLIF($4,''), audio_key=NULLIF($5,'') WHERE id=$1 RETURNING id`,
		v.ID, meaning, v.Example, v.Pronunciation, v.AudioKey).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrVocabNotFound
	}
	return err
}

func (r *PostgresSRSRepo) DeleteVocab(ctx context.Context, vid string) error {
	var id string
	err := r.pool.QueryRow(ctx, `DELETE FROM vocabulary_items WHERE id=$1 RETURNING id`, vid).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrVocabNotFound
	}
	return err
}

func (r *PostgresSRSRepo) LookupVocab(ctx context.Context, language, lemma string) (*domain.Vocab, error) {
	v, err := scanVocab(r.pool.QueryRow(ctx,
		`SELECT `+vocabCols+` FROM vocabulary_items WHERE language=$1 AND item_kind='word' AND lower(lemma)=lower($2)`,
		language, lemma))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrVocabNotFound
	}
	return v, err
}

// easeOrDefault guards against persisting a zero ease (e.g. a legacy card or an
// uninitialized struct) by falling back to the default ease factor.
func easeOrDefault(e float64) float64 {
	if e <= 0 {
		return domain.EaseDefault
	}
	return e
}

func (r *PostgresSRSRepo) LogReview(ctx context.Context, userID string, at time.Time) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO srs_review_log (user_id, reviewed_at) VALUES ($1,$2)`, userID, at.UTC())
	return err
}

func (r *PostgresSRSRepo) ReviewActivity(ctx context.Context, userID string, now time.Time) (today, streak int, err error) {
	nowU := now.UTC()
	if err = r.pool.QueryRow(ctx,
		`SELECT count(*) FROM srs_review_log
		 WHERE user_id=$1 AND (reviewed_at AT TIME ZONE 'UTC')::date = ($2 AT TIME ZONE 'UTC')::date`,
		userID, nowU).Scan(&today); err != nil {
		return 0, 0, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT (reviewed_at AT TIME ZONE 'UTC')::date
		 FROM srs_review_log WHERE user_id=$1 ORDER BY 1 DESC LIMIT 400`, userID)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	days := map[string]bool{}
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return 0, 0, err
		}
		days[d.UTC().Format("2006-01-02")] = true
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	todayStart := time.Date(nowU.Year(), nowU.Month(), nowU.Day(), 0, 0, 0, 0, time.UTC)
	return today, streakFromDays(days, todayStart), nil
}

func (r *PostgresSRSRepo) GetGoal(ctx context.Context, userID string) (int, error) {
	var g int
	err := r.pool.QueryRow(ctx, `SELECT daily_goal FROM srs_settings WHERE user_id=$1`, userID).Scan(&g)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.DailyReviewGoal, nil
		}
		return 0, err
	}
	return g, nil
}

func (r *PostgresSRSRepo) SetGoal(ctx context.Context, userID string, goal int) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO srs_settings (user_id, daily_goal, updated_at) VALUES ($1,$2,now())
		 ON CONFLICT (user_id) DO UPDATE SET daily_goal=EXCLUDED.daily_goal, updated_at=now()`,
		userID, goal)
	return err
}

func (r *PostgresSRSRepo) ReviewWeek(ctx context.Context, userID string, now time.Time) (map[string]int, error) {
	nowU := now.UTC()
	today := time.Date(nowU.Year(), nowU.Month(), nowU.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, -6)
	rows, err := r.pool.Query(ctx,
		`SELECT (reviewed_at AT TIME ZONE 'UTC')::date d, count(*)
		 FROM srs_review_log WHERE user_id=$1 AND reviewed_at >= $2
		 GROUP BY d`, userID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var d time.Time
		var c int
		if err := rows.Scan(&d, &c); err != nil {
			return nil, err
		}
		out[d.UTC().Format("2006-01-02")] = c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresSRSRepo) MyCards(ctx context.Context, userID, query, status string, limit, offset int) ([]domain.CardRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	cond := ""
	switch status {
	case "suspended":
		cond = " AND c.suspended = true"
	case "active":
		cond = " AND c.suspended = false"
	case "leech":
		cond = " AND c.lapses >= " + strconv.Itoa(domain.LeechLapses)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, v.lemma, v.language, v.meaning, c.box, c.interval_days, c.lapses, c.suspended, c.due_at
		 FROM srs_cards c JOIN vocabulary_items v ON v.id = c.vocab_id
		 WHERE c.user_id=$1 AND ($2 = '' OR v.lemma ILIKE '%' || $2 || '%')`+cond+`
		 ORDER BY c.due_at ASC LIMIT $3 OFFSET $4`,
		userID, strings.TrimSpace(query), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CardRow{}
	for rows.Next() {
		var cr domain.CardRow
		var meaning []byte
		if err := rows.Scan(&cr.CardID, &cr.Lemma, &cr.Language, &meaning, &cr.Box, &cr.IntervalDays, &cr.Lapses, &cr.Suspended, &cr.DueAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(meaning, &cr.Meaning)
		cr.Leech = cr.Lapses >= domain.LeechLapses
		out = append(out, cr)
	}
	return out, rows.Err()
}

func (r *PostgresSRSRepo) DeleteCard(ctx context.Context, userID, cardID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM srs_cards WHERE id=$1 AND user_id=$2`, cardID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrCardNotFound
	}
	return nil
}
