package repo

import (
	"context"

	"github.com/leksuu/platform/internal/modules/minimalpairs/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// MemoryRepo serves a curated catalog of minimal-pair contrasts targeting the
// specific English sounds Persian (and Sorani Kurdish) speakers struggle with.
// Content is static (a learning catalog), so no admin CRUD is needed yet.
type MemoryRepo struct{ contrasts []domain.Contrast }

func NewMemoryRepo() *MemoryRepo { return &MemoryRepo{contrasts: seed()} }

func (r *MemoryRepo) List(_ context.Context) ([]domain.Contrast, error) {
	return r.contrasts, nil
}

func (r *MemoryRepo) Get(_ context.Context, id string) (*domain.Contrast, error) {
	for i := range r.contrasts {
		if r.contrasts[i].ID == id {
			c := r.contrasts[i]
			return &c, nil
		}
	}
	return nil, apperr.New("minimal_pairs.not_found", apperr.KindNotFound)
}

func seed() []domain.Contrast {
	return []domain.Contrast{
		{
			ID: "w-v", Label: "W / V", SoundA: "/w/", SoundB: "/v/",
			Tip: map[string]string{
				"fa":  "فارسی صدای /w/ ندارد، برای همین «wet» را مثل «vet» می‌گوییم. برای /w/ لب‌ها را گرد و جلو بده (مثل شروعِ «او») بدون اینکه دندان به لب بخورد. برای /v/ دندان‌های بالا را روی لبِ پایین بگذار و بلرزان.",
				"ckb": "کوردی دەنگی /w/ ی ئینگلیزی بەو شێوەیە نییە؛ بۆ /w/ لێوەکان خڕ بکە بەبێ ئەوەی ددان بەر لێو بکەوێت، بۆ /v/ ددانی سەرەوە بخە سەر لێوی خوارەوە.",
				"en":  "Persian has no /w/, so we say “vet” for “wet”. For /w/ round and push the lips forward (no teeth touching). For /v/ press the upper teeth on the lower lip.",
			},
			Pairs: []domain.Pair{
				{WordA: "wet", WordB: "vet", IpaA: "wɛt", IpaB: "vɛt"},
				{WordA: "west", WordB: "vest", IpaA: "wɛst", IpaB: "vɛst"},
				{WordA: "wine", WordB: "vine", IpaA: "waɪn", IpaB: "vaɪn"},
				{WordA: "worse", WordB: "verse", IpaA: "wɜːrs", IpaB: "vɜːrs"},
				{WordA: "wail", WordB: "veil", IpaA: "weɪl", IpaB: "veɪl"},
				{WordA: "while", WordB: "vile", IpaA: "waɪl", IpaB: "vaɪl"},
			},
		},
		{
			ID: "th-s", Label: "TH / S", SoundA: "/θ/", SoundB: "/s/",
			Tip: map[string]string{
				"fa":  "صدای /θ/ (مثل «think») در فارسی نیست و معمولاً آن را «س» می‌گوییم. برای /θ/ نوکِ زبان را کمی بین دندان‌ها بگذار و هوا را آرام بیرون بده. برای /s/ زبان پشتِ دندان می‌ماند و بیرون نمی‌آید.",
				"ckb": "دەنگی /θ/ لە کوردیدا نییە و زۆرجار دەکرێتە /s/. بۆ /θ/ سەری زمان بخە نێوان ددانەکان و با هەوا بەهێواشی دەربچێت.",
				"en":  "/θ/ (as in “think”) doesn’t exist in Persian; we tend to say /s/. For /θ/ put the tongue tip lightly between the teeth and breathe out. For /s/ the tongue stays behind the teeth.",
			},
			Pairs: []domain.Pair{
				{WordA: "thin", WordB: "sin", IpaA: "θɪn", IpaB: "sɪn"},
				{WordA: "think", WordB: "sink", IpaA: "θɪŋk", IpaB: "sɪŋk"},
				{WordA: "thick", WordB: "sick", IpaA: "θɪk", IpaB: "sɪk"},
				{WordA: "thing", WordB: "sing", IpaA: "θɪŋ", IpaB: "sɪŋ"},
				{WordA: "path", WordB: "pass", IpaA: "pæθ", IpaB: "pæs"},
				{WordA: "mouth", WordB: "mouse", IpaA: "maʊθ", IpaB: "maʊs"},
			},
		},
		{
			ID: "th-t", Label: "TH / T", SoundA: "/θ/", SoundB: "/t/",
			Tip: map[string]string{
				"fa":  "گاهی /θ/ را «ت» می‌گوییم («three» → «tree»). فرقش این است که برای /θ/ زبان نرم بینِ دندان‌هاست و هوا پیوسته بیرون می‌آید، ولی برای /t/ زبان پشتِ دندان جلوی هوا را می‌گیرد و یک‌دفعه رها می‌کند.",
				"ckb": "هەندێک جار /θ/ دەکەینە /t/. بۆ /θ/ زمان نەرمە و لە نێوان ددانە، بەڵام بۆ /t/ زمان لە پشت ددان هەوا ڕادەگرێت.",
				"en":  "We sometimes turn /θ/ into /t/ (“three” → “tree”). For /θ/ the tongue is soft between the teeth with continuous airflow; for /t/ the tongue blocks then releases the air.",
			},
			Pairs: []domain.Pair{
				{WordA: "thin", WordB: "tin", IpaA: "θɪn", IpaB: "tɪn"},
				{WordA: "three", WordB: "tree", IpaA: "θriː", IpaB: "triː"},
				{WordA: "thank", WordB: "tank", IpaA: "θæŋk", IpaB: "tæŋk"},
				{WordA: "thought", WordB: "taught", IpaA: "θɔːt", IpaB: "tɔːt"},
				{WordA: "thread", WordB: "tread", IpaA: "θrɛd", IpaB: "trɛd"},
			},
		},
		{
			ID: "ae-e", Label: "Æ / E", SoundA: "/æ/", SoundB: "/ɛ/",
			Tip: map[string]string{
				"fa":  "صدای /æ/ (مثل «bad») بازتر از «اِ» است؛ دهان را بیشتر باز کن، انگار بینِ «اَ» و «اِ». اگر دهان را کم باز کنی «bed» می‌شود. /ɛ/ همان «اِ»ی معمولی است.",
				"ckb": "دەنگی /æ/ کراوەترە لە /ɛ/؛ دەم زیاتر بکەرەوە، وەک لە نێوان «ا» و «ئە».",
				"en":  "/æ/ (as in “bad”) is more open than /ɛ/; open the mouth wider — between Persian “a” and “e”. Too closed and it becomes “bed”.",
			},
			Pairs: []domain.Pair{
				{WordA: "bad", WordB: "bed", IpaA: "bæd", IpaB: "bɛd"},
				{WordA: "man", WordB: "men", IpaA: "mæn", IpaB: "mɛn"},
				{WordA: "sat", WordB: "set", IpaA: "sæt", IpaB: "sɛt"},
				{WordA: "had", WordB: "head", IpaA: "hæd", IpaB: "hɛd"},
				{WordA: "pan", WordB: "pen", IpaA: "pæn", IpaB: "pɛn"},
				{WordA: "ban", WordB: "Ben", IpaA: "bæn", IpaB: "bɛn"},
			},
		},
		{
			ID: "ih-iy", Label: "I / EE", SoundA: "/ɪ/", SoundB: "/iː/",
			Tip: map[string]string{
				"fa":  "فرقِ مصوتِ کوتاهِ /ɪ/ («ship») با مصوتِ بلندِ /iː/ («sheep»). برای /iː/ صدا را کِش بده و لب‌ها را کمی بیشتر باز/کشیده کن؛ برای /ɪ/ کوتاه و رهاتر بگو.",
				"ckb": "جیاوازی نێوان /ɪ/ ی کورت («ship») و /iː/ ی درێژ («sheep»). بۆ /iː/ دەنگ درێژ بکەرەوە.",
				"en":  "Short /ɪ/ (“ship”) vs long /iː/ (“sheep”). For /iː/ stretch the sound and spread the lips a bit more; /ɪ/ is shorter and more relaxed.",
			},
			Pairs: []domain.Pair{
				{WordA: "ship", WordB: "sheep", IpaA: "ʃɪp", IpaB: "ʃiːp"},
				{WordA: "bit", WordB: "beat", IpaA: "bɪt", IpaB: "biːt"},
				{WordA: "fit", WordB: "feet", IpaA: "fɪt", IpaB: "fiːt"},
				{WordA: "sit", WordB: "seat", IpaA: "sɪt", IpaB: "siːt"},
				{WordA: "live", WordB: "leave", IpaA: "lɪv", IpaB: "liːv"},
				{WordA: "it", WordB: "eat", IpaA: "ɪt", IpaB: "iːt"},
			},
		},
	}
}
