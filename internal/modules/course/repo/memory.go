// Package repo implements the content repository. The in-memory store is shared
// between the course module (read side) and the admin module (write side), so
// authored content immediately appears to learners — mirroring how both hit the
// same tables in Postgres. memory.go = dev/tests; postgres.go (tag pgx) = prod.
package repo

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/leksuu/platform/internal/modules/course/domain"
	"github.com/leksuu/platform/internal/shared/id"
)

type MemoryCourseRepo struct {
	mu           sync.RWMutex
	courses      []domain.Course
	units        map[string][]domain.Unit       // courseID -> units (chapters)
	lessons      map[string][]domain.Lesson     // courseID -> lessons
	items        map[string][]domain.LessonItem // lessonID -> items
	exs          map[string][]domain.Exercise   // itemID -> exercises
	lessonCourse map[string]string              // lessonID -> courseID
	accessRules  map[string]domain.AccessRule   // "type|id" -> rule
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func NewMemoryCourseRepo() *MemoryCourseRepo {
	r := &MemoryCourseRepo{
		units:        map[string][]domain.Unit{},
		lessons:      map[string][]domain.Lesson{},
		items:        map[string][]domain.LessonItem{},
		exs:          map[string][]domain.Exercise{},
		lessonCourse: map[string]string{},
		accessRules:  map[string]domain.AccessRule{},
	}
	r.seed()
	return r
}

func (r *MemoryCourseRepo) seed() {
	const cid = "course-en-a1"
	r.courses = append(r.courses, domain.Course{
		ID: cid, Language: "en", CEFR: "A1", Title: "English for Beginners",
		Description: "Start speaking English from day one.", Version: 1, Published: true,
	})
	const uid = "unit-basics"
	r.units[cid] = []domain.Unit{
		{ID: uid, CourseID: cid, Position: 1, Title: "Getting started", Description: "Your first words in English."},
	}
	r.lessons[cid] = []domain.Lesson{
		{ID: "lesson-greetings", CourseID: cid, UnitID: uid, Position: 1, Title: "Greetings", Version: 1},
		{ID: "lesson-introductions", CourseID: cid, UnitID: uid, Position: 2, Title: "Introductions", Version: 1},
		// An empty checkpoint (no items) — exists in the catalog but has no
		// content yet, so it must not be completable or rewardable (N08).
		{ID: "lesson-empty-cp", CourseID: cid, UnitID: uid, Position: 3, Title: "Checkpoint", Version: 1},
	}
	for _, l := range r.lessons[cid] {
		r.lessonCourse[l.ID] = cid
	}
	r.items["lesson-greetings"] = []domain.LessonItem{
		{ID: "item-g-grammar-ex", LessonID: "lesson-greetings", Position: 7, Kind: domain.KindExercise,
			Payload: raw(`{}`)},
		{ID: "item-g-grammar", LessonID: "lesson-greetings", Position: 6, Kind: domain.KindGrammar,
			Payload: raw(`{"title":"The verb \u201cto be\u201d","rule":{"fa":"فعلِ «to be» یعنی «بودن». برای «I» از am، برای «He/She/It» از is و برای «You/We/They» از are استفاده می‌کنیم.","ckb":"کرداری «to be» واتە «بوون». بۆ «I» am، بۆ «He/She/It» is و بۆ «You/We/They» are بەکاردێنین."},"pattern":"Subject + am / is / are + ...","forms":[{"subject":"I","form":"am"},{"subject":"You / We / They","form":"are"},{"subject":"He / She / It","form":"is"}],"examples":[{"en":"I am happy.","tr":{"fa":"من خوشحال هستم.","ckb":"من دڵخۆشم."}},{"en":"She is my friend.","tr":{"fa":"او دوستِ من است.","ckb":"ئەو هاوڕێمە."}},{"en":"They are students.","tr":{"fa":"آن‌ها دانش‌آموز هستند.","ckb":"ئەوان قوتابین."}}]}`)},
		{ID: "item-g0", LessonID: "lesson-greetings", Position: 1, Kind: domain.KindImage,
			Payload: raw(`{"url":"data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijg2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+SGVsbG8hPC90ZXh0Pgo8dGV4dCB4PSI2MiIgeT0iMjYwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIzNCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC44NSI+SG93IGFyZSB5b3U/PC90ZXh0Pgo8Y2lyY2xlIGN4PSI2NjAiIGN5PSIxNTAiIHI9IjkwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEyIi8+CjxjaXJjbGUgY3g9IjcyMCIgY3k9IjMwMCIgcj0iNTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz4KPC9zdmc+","caption":"Greetings"}`)},
		{ID: "item-g1", LessonID: "lesson-greetings", Position: 2, Kind: domain.KindText,
			Payload: raw(`{"phrase":"Hello! How are you?","translation":{"fa":"سلام! حالت چطوره؟","ckb":"سڵاو! چۆنیت؟"},"example":"Hi Sara! How are you today?","example_translation":{"fa":"سلام سارا! امروز حالت چطوره؟","ckb":"سڵاو سارا! ئەمڕۆ چۆنیت؟"},"note":{"fa":"«How are you?» یک احوال‌پرسیِ رایج است؛ معمولاً جواب می‌دهیم «I’m fine, thanks.»","ckb":"«How are you?» پرسیارێکی باوی حاڵپرسینە؛ زۆرجار وەڵام دەدەینەوە «I’m fine, thanks.»"}}`)},
		{ID: "item-g2", LessonID: "lesson-greetings", Position: 3, Kind: domain.KindMiniStory,
			Payload: raw(`{"narrator":"A man meets his friend on the street. He smiles and says hello. His friend is happy and says hello too.","translation":{"fa":"مردی دوستش را در خیابان می‌بیند. لبخند می‌زند و سلام می‌کند. دوستش خوشحال است و او هم سلام می‌کند.","ckb":"پیاوێک هاوڕێکەی لە شەقامەکە دەبینێت. پێدەکەنێت و سڵاو دەکات. هاوڕێکەی دڵخۆشە و ئەویش سڵاو دەکات."},"qa":[{"q":"Does the man meet a friend?","a":"Yes, he meets a friend."},{"q":"Does he meet a friend or an enemy?","a":"A friend. He meets a friend."},{"q":"Where does he meet his friend?","a":"On the street. He meets him on the street."},{"q":"Does he smile or cry?","a":"He smiles. He smiles and says hello."},{"q":"Is his friend happy?","a":"Yes, his friend is happy to see him."}]}`)},
		{ID: "item-g-pov", LessonID: "lesson-greetings", Position: 5, Kind: domain.KindPOVStory,
			Payload: raw(`{"title":"Meeting a friend","translation":{"fa":"یک داستان در سه زمان: حال، گذشته، آینده.","ckb":"یەک چیرۆک لە سێ کاتدا: ئێستا، ڕابردوو، داهاتوو."},"versions":[{"label":"Present","text":"Every day, a man meets his friend. He smiles and says hello."},{"label":"Past","text":"Yesterday, a man met his friend. He smiled and said hello."},{"label":"Future","text":"Tomorrow, a man will meet his friend. He will smile and say hello."}]}`)},
		{ID: "item-g3", LessonID: "lesson-greetings", Position: 4, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.exs["item-g3"] = []domain.Exercise{
		{ID: "ex-g3-1", LessonItemID: "item-g3", Type: "mcq", Difficulty: 1,
			Prompt:    raw(`{"question":"Choose the greeting","options":["Hello","Apple","Three"]}`),
			AnswerKey: raw(`{"correct":0}`)},
		{ID: "ex-g3-2", LessonItemID: "item-g3", Type: "speak", Difficulty: 1,
			Prompt:    raw(`{"text":"Hello! How are you?","hint":{"fa":"واضح و بلند و شمرده بگو","ckb":"بە ڕوونی و بەرز بیڵێ"}}`),
			AnswerKey: raw(`{"min_score":60}`)},
	}

	r.exs["item-g-grammar-ex"] = []domain.Exercise{
		{ID: "ex-gg-1", LessonItemID: "item-g-grammar-ex", Type: "mcq", Difficulty: 1,
			Prompt:    raw(`{"question":"She ___ my friend.","options":["am","is","are"],"grammar":"to be"}`),
			AnswerKey: raw(`{"correct":1}`)},
		{ID: "ex-gg-2", LessonItemID: "item-g-grammar-ex", Type: "mcq", Difficulty: 1,
			Prompt:    raw(`{"question":"They ___ happy.","options":["am","is","are"],"grammar":"to be"}`),
			AnswerKey: raw(`{"correct":2}`)},
		{ID: "ex-gg-3", LessonItemID: "item-g-grammar-ex", Type: "fill_blank", Difficulty: 1,
			Prompt:    raw(`{"text":"I ___ a teacher.","grammar":"to be"}`),
			AnswerKey: raw(`{"answers":["am"]}`)},
	}

	r.exs["item-routine-grammar-ex"] = []domain.Exercise{
		{ID: "ex-rg-1", LessonItemID: "item-routine-grammar-ex", Type: "mcq", Difficulty: 1,
			Prompt:    raw(`{"question":"She ___ to work.","options":["go","goes","going"],"grammar":"Present Simple"}`),
			AnswerKey: raw(`{"correct":1}`)},
		{ID: "ex-rg-2", LessonItemID: "item-routine-grammar-ex", Type: "mcq", Difficulty: 1,
			Prompt:    raw(`{"question":"They ___ coffee every day.","options":["drink","drinks","drinking"],"grammar":"Present Simple"}`),
			AnswerKey: raw(`{"correct":0}`)},
		{ID: "ex-rg-3", LessonItemID: "item-routine-grammar-ex", Type: "fill_blank", Difficulty: 1,
			Prompt:    raw(`{"text":"I ___ up at seven.","grammar":"Present Simple"}`),
			AnswerKey: raw(`{"answers":["wake"]}`)},
	}
	r.items["lesson-introductions"] = []domain.LessonItem{
		{ID: "item-i1", LessonID: "lesson-introductions", Position: 1, Kind: domain.KindText,
			Payload: raw(`{"phrase":"My name is Sara.","translation":{"fa":"اسم من سارا است.","ckb":"ناوم سارایە."}}`)},
	}
	const euid = "unit-everyday"
	r.units[cid] = append(r.units[cid], domain.Unit{ID: euid, CourseID: cid, Position: 2, Title: "Everyday Life", Description: "Words and phrases for daily life."})
	r.lessons[cid] = append(r.lessons[cid], []domain.Lesson{
		{ID: "lesson-numbers", CourseID: cid, UnitID: euid, Position: 1, Title: "Numbers 1 to 5", Version: 1},
		{ID: "lesson-family", CourseID: cid, UnitID: euid, Position: 2, Title: "Family", Version: 1},
		{ID: "lesson-food", CourseID: cid, UnitID: euid, Position: 3, Title: "Food and Drink", Version: 1},
		{ID: "lesson-routine", CourseID: cid, UnitID: euid, Position: 4, Title: "Daily Routine", Version: 1},
	}...)
	r.exs["item-numbers-4"] = []domain.Exercise{
		{ID: "ex-numbers-1", LessonItemID: "item-numbers-4", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "What is the number 3?", "options": ["three", "five", "eight"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-numbers-2", LessonItemID: "item-numbers-4", Type: "match", Difficulty: 1, Prompt: raw(`{"left": ["1", "2", "3"], "right": ["one", "two", "three"]}`), AnswerKey: raw(`{"pairs": [["1", "one"], ["2", "two"], ["3", "three"]]}`)},
		{ID: "ex-numbers-3", LessonItemID: "item-numbers-4", Type: "fill_blank", Difficulty: 1, Prompt: raw(`{"text": "I have ____ apples. (2)"}`), AnswerKey: raw(`{"answers": ["two", "2"]}`)},
	}
	r.items["lesson-numbers"] = []domain.LessonItem{
		{ID: "item-numbers-1", LessonID: "lesson-numbers", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijc2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+TnVtYmVyczwvdGV4dD4KPHRleHQgeD0iNjIiIHk9IjI1OCIgZm9udC1mYW1pbHk9InNhbnMtc2VyaWYiIGZvbnQtc2l6ZT0iMzIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuODUiPk9uZSwgdHdvLCB0aHJlZTwvdGV4dD4KPGNpcmNsZSBjeD0iNjYwIiBjeT0iMTUwIiByPSI5MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMiIvPgo8Y2lyY2xlIGN4PSI3MjAiIGN5PSIzMDAiIHI9IjUwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEwIi8+PC9zdmc+", "caption": "Numbers"}`)},
		{ID: "item-numbers-2", LessonID: "lesson-numbers", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "One, two, three.", "translation": {"fa": "یک، دو، سه.", "ckb": "یەک، دوو، سێ."}}`)},
		{ID: "item-numbers-3", LessonID: "lesson-numbers", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "I have two books.", "translation": {"fa": "من دو کتاب دارم.", "ckb": "دوو پەرتووکم هەیە."}}`)},
		{ID: "item-numbers-4", LessonID: "lesson-numbers", Position: 4, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.exs["item-family-5"] = []domain.Exercise{
		{ID: "ex-family-1", LessonItemID: "item-family-5", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "How many brothers does Tom have?", "options": ["two", "one", "three"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-family-2", LessonItemID: "item-family-5", Type: "true_false", Difficulty: 1, Prompt: raw(`{"statement": "Tom has one sister."}`), AnswerKey: raw(`{"answer": true}`)},
		{ID: "ex-family-3", LessonItemID: "item-family-5", Type: "reorder", Difficulty: 1, Prompt: raw(`{"words": ["This", "is", "my", "father"]}`), AnswerKey: raw(`{"order": ["This", "is", "my", "father"]}`)},
	}
	r.items["lesson-family"] = []domain.LessonItem{
		{ID: "item-family-1", LessonID: "lesson-family", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijc2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+RmFtaWx5PC90ZXh0Pgo8dGV4dCB4PSI2MiIgeT0iMjU4IiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIzMiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC44NSI+TW90aGVyLCBmYXRoZXIsIGJyb3RoZXI8L3RleHQ+CjxjaXJjbGUgY3g9IjY2MCIgY3k9IjE1MCIgcj0iOTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTIiLz4KPGNpcmNsZSBjeD0iNzIwIiBjeT0iMzAwIiByPSI1MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjwvc3ZnPg==", "caption": "Family"}`)},
		{ID: "item-family-2", LessonID: "lesson-family", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "This is my mother.", "translation": {"fa": "این مادرِ من است.", "ckb": "ئەمە دایکمە."}, "example": "This is my mother. She is very kind.", "example_translation": {"fa": "این مادرِ من است. او خیلی مهربان است.", "ckb": "ئەمە دایکمە. ئەو زۆر دڵۆڤانە."}}`)},
		{ID: "item-family-3", LessonID: "lesson-family", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "He is my brother.", "translation": {"fa": "او برادرِ من است.", "ckb": "ئەو براممە."}}`)},
		{ID: "item-family-4", LessonID: "lesson-family", Position: 4, Kind: domain.KindMiniStory, Payload: raw(`{"narrator": "Tom has a small family. He has one sister and two brothers.", "prompt": "How many brothers does Tom have?", "translation": {"fa": "تام خانواده‌ی کوچکی دارد. او یک خواهر و دو برادر دارد.", "ckb": "تۆم خێزانێکی بچووکی هەیە. خوشکێک و دوو برای هەیە."}}`)},
		{ID: "item-family-5", LessonID: "lesson-family", Position: 5, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.exs["item-food-4"] = []domain.Exercise{
		{ID: "ex-food-1", LessonItemID: "item-food-4", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "Choose a drink.", "options": ["coffee", "table", "chair"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-food-2", LessonItemID: "item-food-4", Type: "fill_blank", Difficulty: 1, Prompt: raw(`{"text": "I ____ tea. (like)"}`), AnswerKey: raw(`{"answers": ["like"]}`)},
		{ID: "ex-food-3", LessonItemID: "item-food-4", Type: "true_false", Difficulty: 1, Prompt: raw(`{"statement": "Water is a drink."}`), AnswerKey: raw(`{"answer": true}`)},
	}
	r.items["lesson-food"] = []domain.LessonItem{
		{ID: "item-food-1", LessonID: "lesson-food", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijc2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+Rm9vZCAmIERyaW5rPC90ZXh0Pgo8dGV4dCB4PSI2MiIgeT0iMjU4IiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIzMiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC44NSI+Q29mZmVlLCB3YXRlciwgYnJlYWQ8L3RleHQ+CjxjaXJjbGUgY3g9IjY2MCIgY3k9IjE1MCIgcj0iOTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTIiLz4KPGNpcmNsZSBjeD0iNzIwIiBjeT0iMzAwIiByPSI1MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjwvc3ZnPg==", "caption": "Food & Drink"}`)},
		{ID: "item-food-2", LessonID: "lesson-food", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "I like coffee.", "translation": {"fa": "من قهوه دوست دارم.", "ckb": "حەز لە قاوە دەکەم."}, "example": "I like coffee, but I don’t like tea.", "example_translation": {"fa": "من قهوه دوست دارم، اما چای دوست ندارم.", "ckb": "حەز لە قاوە دەکەم، بەڵام حەز لە چا ناکەم."}, "note": {"fa": "بعد از «like» معمولاً یک اسم یا فعلِ ing-دار می‌آید.", "ckb": "دوای «like» زۆرجار ناو یان کرداری ing-دار دێت."}}`)},
		{ID: "item-food-3", LessonID: "lesson-food", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "She wants water, please.", "translation": {"fa": "او آب می‌خواهد، لطفا.", "ckb": "ئەو ئاوی دەوێت، تکایە."}}`)},
		{ID: "item-food-4", LessonID: "lesson-food", Position: 4, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.exs["item-routine-5"] = []domain.Exercise{
		{ID: "ex-routine-1", LessonItemID: "item-routine-5", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "What does Sara drink?", "options": ["coffee", "milk", "juice"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-routine-2", LessonItemID: "item-routine-5", Type: "sentence_builder", Difficulty: 1, Prompt: raw(`{"words": ["She", "goes", "to", "work"]}`), AnswerKey: raw(`{"order": ["She", "goes", "to", "work"]}`)},
		{ID: "ex-routine-3", LessonItemID: "item-routine-5", Type: "true_false", Difficulty: 1, Prompt: raw(`{"statement": "Sara wakes up at six."}`), AnswerKey: raw(`{"answer": true}`)},
	}
	r.items["lesson-routine"] = []domain.LessonItem{
		{ID: "item-routine-grammar-ex", LessonID: "lesson-routine", Position: 7, Kind: domain.KindExercise,
			Payload: raw(`{}`)},
		{ID: "item-routine-grammar", LessonID: "lesson-routine", Position: 6, Kind: domain.KindGrammar,
			Payload: raw(`{"title":"Present Simple","rule":{"fa":"برای کارهای روزمره و عادت‌ها از حالِ ساده استفاده می‌کنیم. برای سوم‌شخصِ مفرد (he/she/it) به فعل s اضافه می‌شود.","ckb":"بۆ کاری ڕۆژانە و خووەکان کاتی ئێستای سادە بەکاردێنین. بۆ کەسی سێیەمی تاک (he/she/it) s دەخرێتە سەر کردار."},"pattern":"I / You / We / They + verb   ·   He / She / It + verb + s","forms":[{"subject":"I / You / We / They","form":"wake up"},{"subject":"He / She / It","form":"wakes up"}],"examples":[{"en":"I wake up at seven.","tr":{"fa":"ساعتِ هفت بیدار می‌شوم.","ckb":"لە کاتژمێر حەوت هەڵدەستم."}},{"en":"She goes to work.","tr":{"fa":"او به سرِ کار می‌رود.","ckb":"ئەو دەچێتە سەر کار."}},{"en":"They drink coffee.","tr":{"fa":"آن‌ها قهوه می‌نوشند.","ckb":"ئەوان قاوە دەخۆنەوە."}}]}`)},
		{ID: "item-routine-1", LessonID: "lesson-routine", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPgo8c3RvcCBvZmZzZXQ9IjAiIHN0b3AtY29sb3I9IiM1QTNFMkIiLz48c3RvcCBvZmZzZXQ9IjEiIHN0b3AtY29sb3I9IiNiNDUwM2MiLz48L2xpbmVhckdyYWRpZW50PjwvZGVmcz4KPHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz4KPHRleHQgeD0iNjAiIHk9IjIwMCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9Ijc2IiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+RGFpbHkgUm91dGluZTwvdGV4dD4KPHRleHQgeD0iNjIiIHk9IjI1OCIgZm9udC1mYW1pbHk9InNhbnMtc2VyaWYiIGZvbnQtc2l6ZT0iMzIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuODUiPldha2UgdXAsIHdvcmssIHNsZWVwPC90ZXh0Pgo8Y2lyY2xlIGN4PSI2NjAiIGN5PSIxNTAiIHI9IjkwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEyIi8+CjxjaXJjbGUgY3g9IjcyMCIgY3k9IjMwMCIgcj0iNTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48L3N2Zz4=", "caption": "Daily Routine"}`)},
		{ID: "item-routine-2", LessonID: "lesson-routine", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "I wake up at seven.", "translation": {"fa": "من ساعتِ هفت بیدار می‌شوم.", "ckb": "لە کاتژمێر حەوت لە خەو هەڵدەستم."}, "example": "I wake up at seven and drink water.", "example_translation": {"fa": "ساعتِ هفت بیدار می‌شوم و آب می‌نوشم.", "ckb": "لە کاتژمێر حەوت هەڵدەستم و ئاو دەخۆمەوە."}}`)},
		{ID: "item-routine-3", LessonID: "lesson-routine", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "She goes to work.", "translation": {"fa": "او به سرِ کار می‌رود.", "ckb": "ئەو دەچێتە سەر کار."}}`)},
		{ID: "item-routine-4", LessonID: "lesson-routine", Position: 4, Kind: domain.KindMiniStory, Payload: raw(`{"narrator": "Every day, Sara wakes up at six. She drinks coffee and goes to work.", "prompt": "What does Sara drink?", "translation": {"fa": "هر روز، سارا ساعتِ شش بیدار می‌شود. او قهوه می‌نوشد و به سرِ کار می‌رود.", "ckb": "هەموو ڕۆژێک، سارا لە شەش لە خەو هەڵدەستێت. قاوە دەخواتەوە و دەچێتە سەر کار."}}`)},
		{ID: "item-routine-5", LessonID: "lesson-routine", Position: 5, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.items["lesson-introductions"] = append(r.items["lesson-introductions"], domain.LessonItem{ID: "item-intro-ex", LessonID: "lesson-introductions", Position: 2, Kind: domain.KindExercise, Payload: raw(`{}`)})
	r.exs["item-intro-ex"] = []domain.Exercise{
		{ID: "ex-intro-1", LessonItemID: "item-intro-ex", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "Choose the introduction.", "options": ["My name is Ali.", "Red apple.", "Five dogs."]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-intro-2", LessonItemID: "item-intro-ex", Type: "fill_blank", Difficulty: 1, Prompt: raw(`{"text": "My ____ is Sara. (name)"}`), AnswerKey: raw(`{"answers": ["name"]}`)},
		{ID: "ex-intro-3", LessonItemID: "item-intro-ex", Type: "reorder", Difficulty: 1, Prompt: raw(`{"words": ["I", "am", "from", "Iran"]}`), AnswerKey: raw(`{"order": ["I", "am", "from", "Iran"]}`)},
	}
	for _, l := range r.lessons[cid] {
		r.lessonCourse[l.ID] = cid
	}
	const u3id = "unit-colors"
	r.units[cid] = append(r.units[cid], domain.Unit{ID: u3id, CourseID: cid, Position: 3, Title: "Colors and Describing", Description: "Colors and simple adjectives."})
	r.lessons[cid] = append(r.lessons[cid], []domain.Lesson{
		{ID: "lesson-colors", CourseID: cid, UnitID: u3id, Position: 1, Title: "Colors", Version: 1},
		{ID: "lesson-adjectives", CourseID: cid, UnitID: u3id, Position: 2, Title: "Big and Small", Version: 1},
	}...)
	r.exs["item-colors-4"] = []domain.Exercise{
		{ID: "ex-colors-1", LessonItemID: "item-colors-4", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "What color is the sky usually?", "options": ["blue", "red", "black"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-colors-2", LessonItemID: "item-colors-4", Type: "match", Difficulty: 1, Prompt: raw(`{"left": ["red", "blue", "green"], "right": ["قرمز", "آبی", "سبز"]}`), AnswerKey: raw(`{"pairs": [["red", "قرمز"], ["blue", "آبی"], ["green", "سبز"]]}`)},
		{ID: "ex-colors-3", LessonItemID: "item-colors-4", Type: "fill_blank", Difficulty: 1, Prompt: raw(`{"text": "Grass is ____ . (green)"}`), AnswerKey: raw(`{"answers": ["green"]}`)},
	}
	r.items["lesson-colors"] = []domain.LessonItem{
		{ID: "item-colors-1", LessonID: "lesson-colors", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPjxzdG9wIG9mZnNldD0iMCIgc3RvcC1jb2xvcj0iIzVBM0UyQiIvPjxzdG9wIG9mZnNldD0iMSIgc3RvcC1jb2xvcj0iI2I0NTAzYyIvPjwvbGluZWFyR3JhZGllbnQ+PC9kZWZzPgo8cmVjdCB3aWR0aD0iODAwIiBoZWlnaHQ9IjM4MCIgZmlsbD0idXJsKCNnKSIvPgo8dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5Db2xvcnM8L3RleHQ+Cjx0ZXh0IHg9IjYyIiB5PSIyNTgiIGZvbnQtZmFtaWx5PSJzYW5zLXNlcmlmIiBmb250LXNpemU9IjMyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjg1Ij5SZWQsIGJsdWUsIGdyZWVuPC90ZXh0Pgo8Y2lyY2xlIGN4PSI2NjAiIGN5PSIxNTAiIHI9IjkwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEyIi8+PGNpcmNsZSBjeD0iNzIwIiBjeT0iMzAwIiByPSI1MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjwvc3ZnPg==", "caption": "Colors"}`)},
		{ID: "item-colors-2", LessonID: "lesson-colors", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "The apple is red.", "translation": {"fa": "سیب قرمز است.", "ckb": "سێوەکە سوورە."}}`)},
		{ID: "item-colors-3", LessonID: "lesson-colors", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "The sky is blue.", "translation": {"fa": "آسمان آبی است.", "ckb": "ئاسمان شینە."}}`)},
		{ID: "item-colors-4", LessonID: "lesson-colors", Position: 4, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.exs["item-adjectives-4"] = []domain.Exercise{
		{ID: "ex-adjectives-1", LessonItemID: "item-adjectives-4", Type: "true_false", Difficulty: 1, Prompt: raw(`{"statement": "An elephant is small."}`), AnswerKey: raw(`{"answer": false}`)},
		{ID: "ex-adjectives-2", LessonItemID: "item-adjectives-4", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "Which word is the opposite of big?", "options": ["small", "tall", "red"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-adjectives-3", LessonItemID: "item-adjectives-4", Type: "reorder", Difficulty: 1, Prompt: raw(`{"words": ["The", "car", "is", "fast"]}`), AnswerKey: raw(`{"order": ["The", "car", "is", "fast"]}`)},
	}
	r.items["lesson-adjectives"] = []domain.LessonItem{
		{ID: "item-adjectives-1", LessonID: "lesson-adjectives", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPjxzdG9wIG9mZnNldD0iMCIgc3RvcC1jb2xvcj0iIzVBM0UyQiIvPjxzdG9wIG9mZnNldD0iMSIgc3RvcC1jb2xvcj0iI2I0NTAzYyIvPjwvbGluZWFyR3JhZGllbnQ+PC9kZWZzPgo8cmVjdCB3aWR0aD0iODAwIiBoZWlnaHQ9IjM4MCIgZmlsbD0idXJsKCNnKSIvPgo8dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5PcHBvc2l0ZXM8L3RleHQ+Cjx0ZXh0IHg9IjYyIiB5PSIyNTgiIGZvbnQtZmFtaWx5PSJzYW5zLXNlcmlmIiBmb250LXNpemU9IjMyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjg1Ij5CaWcgYW5kIHNtYWxsPC90ZXh0Pgo8Y2lyY2xlIGN4PSI2NjAiIGN5PSIxNTAiIHI9IjkwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEyIi8+PGNpcmNsZSBjeD0iNzIwIiBjeT0iMzAwIiByPSI1MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjwvc3ZnPg==", "caption": "Opposites"}`)},
		{ID: "item-adjectives-2", LessonID: "lesson-adjectives", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "The elephant is big.", "translation": {"fa": "فیل بزرگ است.", "ckb": "فیلەکە گەورەیە."}}`)},
		{ID: "item-adjectives-3", LessonID: "lesson-adjectives", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "The mouse is small.", "translation": {"fa": "موش کوچک است.", "ckb": "مشکەکە بچووکە."}}`)},
		{ID: "item-adjectives-4", LessonID: "lesson-adjectives", Position: 4, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	const u4id = "unit-town"
	r.units[cid] = append(r.units[cid], domain.Unit{ID: u4id, CourseID: cid, Position: 4, Title: "Out and About", Description: "Time, shopping and directions."})
	r.lessons[cid] = append(r.lessons[cid], []domain.Lesson{
		{ID: "lesson-time", CourseID: cid, UnitID: u4id, Position: 1, Title: "Time", Version: 1},
		{ID: "lesson-shopping", CourseID: cid, UnitID: u4id, Position: 2, Title: "Shopping", Version: 1},
		{ID: "lesson-directions", CourseID: cid, UnitID: u4id, Position: 3, Title: "Directions", Version: 1},
	}...)
	r.exs["item-time-4"] = []domain.Exercise{
		{ID: "ex-time-1", LessonItemID: "item-time-4", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "Which is a time word?", "options": ["hour", "apple", "green"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-time-2", LessonItemID: "item-time-4", Type: "fill_blank", Difficulty: 1, Prompt: raw(`{"text": "It is ____ . (seven)"}`), AnswerKey: raw(`{"answers": ["seven", "7"]}`)},
		{ID: "ex-time-3", LessonItemID: "item-time-4", Type: "true_false", Difficulty: 1, Prompt: raw(`{"statement": "A clock shows the time."}`), AnswerKey: raw(`{"answer": true}`)},
	}
	r.items["lesson-time"] = []domain.LessonItem{
		{ID: "item-time-1", LessonID: "lesson-time", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPjxzdG9wIG9mZnNldD0iMCIgc3RvcC1jb2xvcj0iIzVBM0UyQiIvPjxzdG9wIG9mZnNldD0iMSIgc3RvcC1jb2xvcj0iI2I0NTAzYyIvPjwvbGluZWFyR3JhZGllbnQ+PC9kZWZzPgo8cmVjdCB3aWR0aD0iODAwIiBoZWlnaHQ9IjM4MCIgZmlsbD0idXJsKCNnKSIvPgo8dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5UaW1lPC90ZXh0Pgo8dGV4dCB4PSI2MiIgeT0iMjU4IiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIzMiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC44NSI+V2hhdCB0aW1lIGlzIGl0PzwvdGV4dD4KPGNpcmNsZSBjeD0iNjYwIiBjeT0iMTUwIiByPSI5MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMiIvPjxjaXJjbGUgY3g9IjcyMCIgY3k9IjMwMCIgcj0iNTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48L3N2Zz4=", "caption": "Time"}`)},
		{ID: "item-time-2", LessonID: "lesson-time", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "What time is it?", "translation": {"fa": "ساعت چند است؟", "ckb": "کاتژمێر چەندە؟"}}`)},
		{ID: "item-time-3", LessonID: "lesson-time", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "It is seven.", "translation": {"fa": "ساعت هفت است.", "ckb": "کاتژمێر حەوتە."}}`)},
		{ID: "item-time-4", LessonID: "lesson-time", Position: 4, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.exs["item-shopping-5"] = []domain.Exercise{
		{ID: "ex-shopping-1", LessonItemID: "item-shopping-5", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "What does Mary buy?", "options": ["bread and milk", "a car", "shoes"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-shopping-2", LessonItemID: "item-shopping-5", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "How do you ask the price?", "options": ["How much is it?", "What is your name?", "Where are you?"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-shopping-3", LessonItemID: "item-shopping-5", Type: "sentence_builder", Difficulty: 1, Prompt: raw(`{"words": ["It", "is", "five", "dollars"]}`), AnswerKey: raw(`{"order": ["It", "is", "five", "dollars"]}`)},
	}
	r.items["lesson-shopping"] = []domain.LessonItem{
		{ID: "item-shopping-1", LessonID: "lesson-shopping", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPjxzdG9wIG9mZnNldD0iMCIgc3RvcC1jb2xvcj0iIzVBM0UyQiIvPjxzdG9wIG9mZnNldD0iMSIgc3RvcC1jb2xvcj0iI2I0NTAzYyIvPjwvbGluZWFyR3JhZGllbnQ+PC9kZWZzPgo8cmVjdCB3aWR0aD0iODAwIiBoZWlnaHQ9IjM4MCIgZmlsbD0idXJsKCNnKSIvPgo8dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5TaG9wcGluZzwvdGV4dD4KPHRleHQgeD0iNjIiIHk9IjI1OCIgZm9udC1mYW1pbHk9InNhbnMtc2VyaWYiIGZvbnQtc2l6ZT0iMzIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuODUiPkhvdyBtdWNoIGlzIGl0PzwvdGV4dD4KPGNpcmNsZSBjeD0iNjYwIiBjeT0iMTUwIiByPSI5MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMiIvPjxjaXJjbGUgY3g9IjcyMCIgY3k9IjMwMCIgcj0iNTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48L3N2Zz4=", "caption": "Shopping"}`)},
		{ID: "item-shopping-2", LessonID: "lesson-shopping", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "How much is it?", "translation": {"fa": "قیمتش چند است؟", "ckb": "بەندە بەچەندە؟"}}`)},
		{ID: "item-shopping-3", LessonID: "lesson-shopping", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "It is five dollars.", "translation": {"fa": "پنج دلار است.", "ckb": "پێنج دۆلارە."}}`)},
		{ID: "item-shopping-4", LessonID: "lesson-shopping", Position: 4, Kind: domain.KindMiniStory, Payload: raw(`{"narrator": "Mary goes to the shop. She buys bread and milk.", "prompt": "What does Mary buy?", "translation": {"fa": "ماری به مغازه می‌رود. او نان و شیر می‌خرد.", "ckb": "ماری دەچێتە دوکانەکە. نان و شیر دەکڕێت."}}`)},
		{ID: "item-shopping-5", LessonID: "lesson-shopping", Position: 5, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.exs["item-directions-4"] = []domain.Exercise{
		{ID: "ex-directions-1", LessonItemID: "item-directions-4", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "Which word is the opposite of left?", "options": ["right", "up", "big"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-directions-2", LessonItemID: "item-directions-4", Type: "match", Difficulty: 1, Prompt: raw(`{"left": ["left", "right", "up"], "right": ["چپ", "راست", "بالا"]}`), AnswerKey: raw(`{"pairs": [["left", "چپ"], ["right", "راست"], ["up", "بالا"]]}`)},
		{ID: "ex-directions-3", LessonItemID: "item-directions-4", Type: "true_false", Difficulty: 1, Prompt: raw(`{"statement": "You can turn left or right."}`), AnswerKey: raw(`{"answer": true}`)},
	}
	r.items["lesson-directions"] = []domain.LessonItem{
		{ID: "item-directions-1", LessonID: "lesson-directions", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+CjxkZWZzPjxsaW5lYXJHcmFkaWVudCBpZD0iZyIgeDE9IjAiIHkxPSIwIiB4Mj0iMSIgeTI9IjEiPjxzdG9wIG9mZnNldD0iMCIgc3RvcC1jb2xvcj0iIzVBM0UyQiIvPjxzdG9wIG9mZnNldD0iMSIgc3RvcC1jb2xvcj0iI2I0NTAzYyIvPjwvbGluZWFyR3JhZGllbnQ+PC9kZWZzPgo8cmVjdCB3aWR0aD0iODAwIiBoZWlnaHQ9IjM4MCIgZmlsbD0idXJsKCNnKSIvPgo8dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5EaXJlY3Rpb25zPC90ZXh0Pgo8dGV4dCB4PSI2MiIgeT0iMjU4IiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIzMiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC44NSI+TGVmdCBhbmQgcmlnaHQ8L3RleHQ+CjxjaXJjbGUgY3g9IjY2MCIgY3k9IjE1MCIgcj0iOTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTIiLz48Y2lyY2xlIGN4PSI3MjAiIGN5PSIzMDAiIHI9IjUwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEwIi8+PC9zdmc+", "caption": "Directions"}`)},
		{ID: "item-directions-2", LessonID: "lesson-directions", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "Turn left.", "translation": {"fa": "به چپ بپیچ.", "ckb": "بەلای چەپدا بسووڕێوە."}}`)},
		{ID: "item-directions-3", LessonID: "lesson-directions", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "The shop is on the right.", "translation": {"fa": "مغازه سمتِ راست است.", "ckb": "دوکانەکە لای ڕاستە."}}`)},
		{ID: "item-directions-4", LessonID: "lesson-directions", Position: 4, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	for _, l := range r.lessons[cid] {
		r.lessonCourse[l.ID] = cid
	}
	const u5id = "unit-weather"
	r.units[cid] = append(r.units[cid], domain.Unit{ID: u5id, CourseID: cid, Position: 5, Title: "Days and Weather", Description: "Days of the week and the weather."})
	r.lessons[cid] = append(r.lessons[cid], []domain.Lesson{
		{ID: "lesson-days", CourseID: cid, UnitID: u5id, Position: 1, Title: "Days of the Week", Version: 1},
		{ID: "lesson-weather", CourseID: cid, UnitID: u5id, Position: 2, Title: "Weather", Version: 1},
	}...)
	r.exs["item-days-4"] = []domain.Exercise{
		{ID: "ex-days-1", LessonItemID: "item-days-4", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "Which one is a day of the week?", "options": ["Monday", "Apple", "Blue"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-days-2", LessonItemID: "item-days-4", Type: "reorder", Difficulty: 1, Prompt: raw(`{"words": ["Today", "is", "Monday"]}`), AnswerKey: raw(`{"order": ["Today", "is", "Monday"]}`)},
		{ID: "ex-days-3", LessonItemID: "item-days-4", Type: "fill_blank", Difficulty: 1, Prompt: raw(`{"text": "There are seven days in a ____ . (week)"}`), AnswerKey: raw(`{"answers": ["week"]}`)},
	}
	r.items["lesson-days"] = []domain.LessonItem{
		{ID: "item-days-1", LessonID: "lesson-days", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjYjQ1MDNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5EYXlzPC90ZXh0Pjx0ZXh0IHg9IjYyIiB5PSIyNTgiIGZvbnQtZmFtaWx5PSJzYW5zLXNlcmlmIiBmb250LXNpemU9IjMyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjg1Ij5Nb25kYXkgdG8gU3VuZGF5PC90ZXh0PjxjaXJjbGUgY3g9IjY2MCIgY3k9IjE1MCIgcj0iOTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTIiLz48Y2lyY2xlIGN4PSI3MjAiIGN5PSIzMDAiIHI9IjUwIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEwIi8+PC9zdmc+", "caption": "Days"}`)},
		{ID: "item-days-2", LessonID: "lesson-days", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "Today is Monday.", "translation": {"fa": "امروز دوشنبه است.", "ckb": "ئەمڕۆ دووشەممەیە."}}`)},
		{ID: "item-days-3", LessonID: "lesson-days", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "Tomorrow is Tuesday.", "translation": {"fa": "فردا سه‌شنبه است.", "ckb": "سبەینێ سێشەممەیە."}}`)},
		{ID: "item-days-4", LessonID: "lesson-days", Position: 4, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	r.exs["item-weather-5"] = []domain.Exercise{
		{ID: "ex-weather-1", LessonItemID: "item-weather-5", Type: "mcq", Difficulty: 1, Prompt: raw(`{"question": "What does Sara take?", "options": ["an umbrella", "a book", "a dog"]}`), AnswerKey: raw(`{"correct": 0}`)},
		{ID: "ex-weather-2", LessonItemID: "item-weather-5", Type: "true_false", Difficulty: 1, Prompt: raw(`{"statement": "It is hot in winter."}`), AnswerKey: raw(`{"answer": false}`)},
		{ID: "ex-weather-3", LessonItemID: "item-weather-5", Type: "match", Difficulty: 1, Prompt: raw(`{"left": ["hot", "cold", "rain"], "right": ["گرم", "سرد", "باران"]}`), AnswerKey: raw(`{"pairs": [["hot", "گرم"], ["cold", "سرد"], ["rain", "باران"]]}`)},
	}
	r.items["lesson-weather"] = []domain.LessonItem{
		{ID: "item-weather-1", LessonID: "lesson-weather", Position: 1, Kind: domain.KindImage, Payload: raw(`{"url": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjYjQ1MDNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMjAwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzYiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5XZWF0aGVyPC90ZXh0Pjx0ZXh0IHg9IjYyIiB5PSIyNTgiIGZvbnQtZmFtaWx5PSJzYW5zLXNlcmlmIiBmb250LXNpemU9IjMyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjg1Ij5TdW4sIHJhaW4sIHNub3c8L3RleHQ+PGNpcmNsZSBjeD0iNjYwIiBjeT0iMTUwIiByPSI5MCIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMiIvPjxjaXJjbGUgY3g9IjcyMCIgY3k9IjMwMCIgcj0iNTAiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48L3N2Zz4=", "caption": "Weather"}`)},
		{ID: "item-weather-2", LessonID: "lesson-weather", Position: 2, Kind: domain.KindText, Payload: raw(`{"phrase": "It is hot today.", "translation": {"fa": "امروز گرم است.", "ckb": "ئەمڕۆ گەرمە."}}`)},
		{ID: "item-weather-3", LessonID: "lesson-weather", Position: 3, Kind: domain.KindText, Payload: raw(`{"phrase": "It is cold in winter.", "translation": {"fa": "در زمستان سرد است.", "ckb": "لە زستاندا ساردە."}}`)},
		{ID: "item-weather-4", LessonID: "lesson-weather", Position: 4, Kind: domain.KindMiniStory, Payload: raw(`{"narrator": "Today it is rainy. Sara takes her umbrella and goes to work.", "prompt": "What does Sara take?", "translation": {"fa": "امروز بارانی است. سارا چترش را برمی‌دارد و به سرِ کار می‌رود.", "ckb": "ئەمڕۆ بارانییە. سارا چەترەکەی هەڵدەگرێت و دەچێتە سەر کار."}}`)},
		{ID: "item-weather-5", LessonID: "lesson-weather", Position: 5, Kind: domain.KindExercise, Payload: raw(`{}`)},
	}
	for _, l := range r.lessons[cid] {
		r.lessonCourse[l.ID] = cid
	}
	type lmeta struct {
		cover, focus string
		obj          json.RawMessage
	}
	meta := map[string]lmeta{
		"lesson-greetings":     {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjYjQ1MDNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+U0FZIEhFTExPPC90ZXh0Pjx0ZXh0IHg9IjU4IiB5PSIyNDAiIGZvbnQtZmFtaWx5PSJHZW9yZ2lhLCBzZXJpZiIgZm9udC1zaXplPSI3MiIgZmlsbD0iI0ZERkFGNiIgZm9udC13ZWlnaHQ9ImJvbGQiPkdyZWV0aW5nczwvdGV4dD48Y2lyY2xlIGN4PSI2NzAiIGN5PSIxNDAiIHI9IjkyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEwIi8+PGNpcmNsZSBjeD0iNzMwIiBjeT0iMzAwIiByPSI1MiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4wOCIvPjwvc3ZnPg==", focus: "phrases", obj: raw(`{"summary": {"fa": "سلام و احوال‌پرسیِ ساده", "ckb": "سڵاوکردن و حاڵپرسینی سادە"}, "points": [{"fa": "سلام‌کردن به دیگران", "ckb": "سڵاوکردن لە کەسانی تر"}, {"fa": "پرسیدنِ حالِ کسی", "ckb": "حاڵپرسین لە کەسێک"}]}`)},
		"lesson-introductions": {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjYjQ1MDNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+SU5UUk9EVUNFIFlPVVJTRUxGPC90ZXh0Pjx0ZXh0IHg9IjU4IiB5PSIyNDAiIGZvbnQtZmFtaWx5PSJHZW9yZ2lhLCBzZXJpZiIgZm9udC1zaXplPSI3MiIgZmlsbD0iI0ZERkFGNiIgZm9udC13ZWlnaHQ9ImJvbGQiPkludHJvZHVjdGlvbnM8L3RleHQ+PGNpcmNsZSBjeD0iNjcwIiBjeT0iMTQwIiByPSI5MiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjxjaXJjbGUgY3g9IjczMCIgY3k9IjMwMCIgcj0iNTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMDgiLz48L3N2Zz4=", focus: "phrases", obj: raw(`{"summary": {"fa": "معرفیِ خودت", "ckb": "ناساندنی خۆت"}, "points": [{"fa": "گفتنِ نامت", "ckb": "ناوی خۆت بڵێیت"}, {"fa": "گفتنِ اهلِ کجایی", "ckb": "بڵێیت خەڵکی کوێیت"}]}`)},
		"lesson-numbers":       {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjOGE1YTNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+Q09VTlRJTkc8L3RleHQ+PHRleHQgeD0iNTgiIHk9IjI0MCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9IjcyIiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+TnVtYmVycyAxIHRvIDU8L3RleHQ+PGNpcmNsZSBjeD0iNjcwIiBjeT0iMTQwIiByPSI5MiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjxjaXJjbGUgY3g9IjczMCIgY3k9IjMwMCIgcj0iNTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMDgiLz48L3N2Zz4=", focus: "vocabulary", obj: raw(`{"summary": {"fa": "اعدادِ ۱ تا ۵ و شمارش", "ckb": "ژمارەکانی ١ بۆ ٥ و ژماردن"}, "points": [{"fa": "اعدادِ یک تا پنج", "ckb": "ژمارە یەک تا پێنج"}, {"fa": "گفتنِ تعداد", "ckb": "گوتنی ژمارە"}]}`)},
		"lesson-family":        {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjOGE1YTNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+WU9VUiBGQU1JTFk8L3RleHQ+PHRleHQgeD0iNTgiIHk9IjI0MCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9IjcyIiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+RmFtaWx5PC90ZXh0PjxjaXJjbGUgY3g9IjY3MCIgY3k9IjE0MCIgcj0iOTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48Y2lyY2xlIGN4PSI3MzAiIGN5PSIzMDAiIHI9IjUyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjA4Ii8+PC9zdmc+", focus: "vocabulary", obj: raw(`{"summary": {"fa": "کلماتِ خانواده", "ckb": "وشەکانی خێزان"}, "points": [{"fa": "اعضای خانواده", "ckb": "ئەندامانی خێزان"}, {"fa": "صحبت درباره‌ی خانواده", "ckb": "قسەکردن دەربارەی خێزان"}]}`)},
		"lesson-food":          {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjOGE1YTNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+Rk9PRCAmIERSSU5LPC90ZXh0Pjx0ZXh0IHg9IjU4IiB5PSIyNDAiIGZvbnQtZmFtaWx5PSJHZW9yZ2lhLCBzZXJpZiIgZm9udC1zaXplPSI3MiIgZmlsbD0iI0ZERkFGNiIgZm9udC13ZWlnaHQ9ImJvbGQiPkZvb2QgYW5kIERyaW5rPC90ZXh0PjxjaXJjbGUgY3g9IjY3MCIgY3k9IjE0MCIgcj0iOTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48Y2lyY2xlIGN4PSI3MzAiIGN5PSIzMDAiIHI9IjUyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjA4Ii8+PC9zdmc+", focus: "vocabulary", obj: raw(`{"summary": {"fa": "کلماتِ غذا و نوشیدنی", "ckb": "وشەکانی خواردن و خواردنەوە"}, "points": [{"fa": "نام‌های خوراکی", "ckb": "ناوی خۆراکەکان"}, {"fa": "گفتنِ آنچه دوست داری", "ckb": "گوتنی ئەوەی حەزی لێدەکەیت"}]}`)},
		"lesson-routine":       {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjM2Q0YTVhIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+UFJFU0VOVCBTSU1QTEU8L3RleHQ+PHRleHQgeD0iNTgiIHk9IjI0MCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9IjcyIiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+RGFpbHkgUm91dGluZTwvdGV4dD48Y2lyY2xlIGN4PSI2NzAiIGN5PSIxNDAiIHI9IjkyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEwIi8+PGNpcmNsZSBjeD0iNzMwIiBjeT0iMzAwIiByPSI1MiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4wOCIvPjwvc3ZnPg==", focus: "grammar", obj: raw(`{"summary": {"fa": "زمانِ حالِ ساده برای کارهای روزانه", "ckb": "کاتی ئێستای سادە بۆ کارە ڕۆژانەکان"}, "points": [{"fa": "ساختِ جمله با حالِ ساده", "ckb": "دروستکردنی ڕستە بە کاتی ئێستای سادە"}, {"fa": "کارهای هرروزه", "ckb": "کارە ڕۆژانەکان"}], "tip": {"fa": "برای کارهای هرروزه از زمانِ حالِ ساده استفاده می‌کنیم. برای he/she به فعل s اضافه می‌شود: She drinks coffee.", "ckb": "بۆ کارە ڕۆژانەکان کاتی ئێستای سادە بەکاردێت. بۆ he/she ئەو s بۆ کردار زیاد دەکرێت: She drinks coffee."}}`)},
		"lesson-colors":        {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjOGE1YTNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+Q09MT1JTPC90ZXh0Pjx0ZXh0IHg9IjU4IiB5PSIyNDAiIGZvbnQtZmFtaWx5PSJHZW9yZ2lhLCBzZXJpZiIgZm9udC1zaXplPSI3MiIgZmlsbD0iI0ZERkFGNiIgZm9udC13ZWlnaHQ9ImJvbGQiPkNvbG9yczwvdGV4dD48Y2lyY2xlIGN4PSI2NzAiIGN5PSIxNDAiIHI9IjkyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjEwIi8+PGNpcmNsZSBjeD0iNzMwIiBjeT0iMzAwIiByPSI1MiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4wOCIvPjwvc3ZnPg==", focus: "vocabulary", obj: raw(`{"summary": {"fa": "نام‌های رنگ‌ها", "ckb": "ناوی ڕەنگەکان"}, "points": [{"fa": "رنگ‌های اصلی", "ckb": "ڕەنگە سەرەکییەکان"}, {"fa": "توصیفِ چیزها با رنگ", "ckb": "وەسفکردنی شت بە ڕەنگ"}]}`)},
		"lesson-adjectives":    {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjM2Q0YTVhIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+QURKRUNUSVZFUzwvdGV4dD48dGV4dCB4PSI1OCIgeT0iMjQwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzIiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5CaWcgYW5kIFNtYWxsPC90ZXh0PjxjaXJjbGUgY3g9IjY3MCIgY3k9IjE0MCIgcj0iOTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48Y2lyY2xlIGN4PSI3MzAiIGN5PSIzMDAiIHI9IjUyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjA4Ii8+PC9zdmc+", focus: "grammar", obj: raw(`{"summary": {"fa": "صفت‌ها و متضادها", "ckb": "ئادیێکتیڤ و پێچەوانەکان"}, "points": [{"fa": "صفت‌های اندازه", "ckb": "ئادیێکتیڤی قەبارە"}, {"fa": "متضادها مثل بزرگ/کوچک", "ckb": "پێچەوانە وەک گەورە/بچووک"}], "tip": {"fa": "صفت معمولاً قبل از اسم می‌آید: a big house. صفت‌ها در انگلیسی جمع/مفرد نمی‌شوند.", "ckb": "ئادیێکتیڤ زۆرجار پێش ناو دێت: a big house. ئادیێکتیڤەکان لە ئینگلیزیدا کۆ ناکرێن."}}`)},
		"lesson-time":          {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjOGE1YTNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+VEVMTElORyBUSU1FPC90ZXh0Pjx0ZXh0IHg9IjU4IiB5PSIyNDAiIGZvbnQtZmFtaWx5PSJHZW9yZ2lhLCBzZXJpZiIgZm9udC1zaXplPSI3MiIgZmlsbD0iI0ZERkFGNiIgZm9udC13ZWlnaHQ9ImJvbGQiPlRpbWU8L3RleHQ+PGNpcmNsZSBjeD0iNjcwIiBjeT0iMTQwIiByPSI5MiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjxjaXJjbGUgY3g9IjczMCIgY3k9IjMwMCIgcj0iNTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMDgiLz48L3N2Zz4=", focus: "vocabulary", obj: raw(`{"summary": {"fa": "پرسیدن و گفتنِ ساعت", "ckb": "پرسین و گوتنی کات"}, "points": [{"fa": "کلماتِ زمان", "ckb": "وشەکانی کات"}, {"fa": "پرسیدنِ ساعت چند است", "ckb": "پرسینی کاتژمێر چەندە"}]}`)},
		"lesson-shopping":      {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjYjQ1MDNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+U0hPUFBJTkc8L3RleHQ+PHRleHQgeD0iNTgiIHk9IjI0MCIgZm9udC1mYW1pbHk9Ikdlb3JnaWEsIHNlcmlmIiBmb250LXNpemU9IjcyIiBmaWxsPSIjRkRGQUY2IiBmb250LXdlaWdodD0iYm9sZCI+U2hvcHBpbmc8L3RleHQ+PGNpcmNsZSBjeD0iNjcwIiBjeT0iMTQwIiByPSI5MiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC4xMCIvPjxjaXJjbGUgY3g9IjczMCIgY3k9IjMwMCIgcj0iNTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMDgiLz48L3N2Zz4=", focus: "phrases", obj: raw(`{"summary": {"fa": "عبارت‌های خرید و قیمت", "ckb": "دەستەواژەکانی کڕین و نرخ"}, "points": [{"fa": "پرسیدنِ قیمت", "ckb": "پرسینی نرخ"}, {"fa": "خریدِ چیزها", "ckb": "کڕینی شتەکان"}]}`)},
		"lesson-directions":    {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjOGE1YTNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+RElSRUNUSU9OUzwvdGV4dD48dGV4dCB4PSI1OCIgeT0iMjQwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzIiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5EaXJlY3Rpb25zPC90ZXh0PjxjaXJjbGUgY3g9IjY3MCIgY3k9IjE0MCIgcj0iOTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48Y2lyY2xlIGN4PSI3MzAiIGN5PSIzMDAiIHI9IjUyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjA4Ii8+PC9zdmc+", focus: "vocabulary", obj: raw(`{"summary": {"fa": "جهت‌ها: چپ و راست", "ckb": "ئاراستەکان: چەپ و ڕاست"}, "points": [{"fa": "کلماتِ جهت", "ckb": "وشەکانی ئاراستە"}, {"fa": "پیداکردنِ مکان", "ckb": "دۆزینەوەی شوێن"}]}`)},
		"lesson-days":          {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjOGE1YTNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+REFZUzwvdGV4dD48dGV4dCB4PSI1OCIgeT0iMjQwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzIiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5EYXlzIG9mIHRoZSBXZWVrPC90ZXh0PjxjaXJjbGUgY3g9IjY3MCIgY3k9IjE0MCIgcj0iOTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48Y2lyY2xlIGN4PSI3MzAiIGN5PSIzMDAiIHI9IjUyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjA4Ii8+PC9zdmc+", focus: "vocabulary", obj: raw(`{"summary": {"fa": "روزهای هفته", "ckb": "ڕۆژانی هەفتە"}, "points": [{"fa": "نام‌های روزها", "ckb": "ناوی ڕۆژەکان"}, {"fa": "امروز و فردا", "ckb": "ئەمڕۆ و سبەینێ"}]}`)},
		"lesson-weather":       {cover: "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI4MDAiIGhlaWdodD0iMzgwIiB2aWV3Qm94PSIwIDAgODAwIDM4MCI+PGRlZnM+PGxpbmVhckdyYWRpZW50IGlkPSJnIiB4MT0iMCIgeTE9IjAiIHgyPSIxIiB5Mj0iMSI+PHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjNUEzRTJCIi8+PHN0b3Agb2Zmc2V0PSIxIiBzdG9wLWNvbG9yPSIjOGE1YTNjIi8+PC9saW5lYXJHcmFkaWVudD48L2RlZnM+PHJlY3Qgd2lkdGg9IjgwMCIgaGVpZ2h0PSIzODAiIGZpbGw9InVybCgjZykiLz48dGV4dCB4PSI2MCIgeT0iMTUwIiBmb250LWZhbWlseT0ic2Fucy1zZXJpZiIgZm9udC1zaXplPSIyNiIgZmlsbD0iI0ZERkFGNiIgb3BhY2l0eT0iMC43IiBsZXR0ZXItc3BhY2luZz0iMyI+V0VBVEhFUjwvdGV4dD48dGV4dCB4PSI1OCIgeT0iMjQwIiBmb250LWZhbWlseT0iR2VvcmdpYSwgc2VyaWYiIGZvbnQtc2l6ZT0iNzIiIGZpbGw9IiNGREZBRjYiIGZvbnQtd2VpZ2h0PSJib2xkIj5XZWF0aGVyPC90ZXh0PjxjaXJjbGUgY3g9IjY3MCIgY3k9IjE0MCIgcj0iOTIiIGZpbGw9IiNGREZBRjYiIG9wYWNpdHk9IjAuMTAiLz48Y2lyY2xlIGN4PSI3MzAiIGN5PSIzMDAiIHI9IjUyIiBmaWxsPSIjRkRGQUY2IiBvcGFjaXR5PSIwLjA4Ii8+PC9zdmc+", focus: "vocabulary", obj: raw(`{"summary": {"fa": "کلماتِ آب‌وهوا", "ckb": "وشەکانی کەشوهەوا"}, "points": [{"fa": "توصیفِ هوا", "ckb": "وەسفی کەشوهەوا"}, {"fa": "گرم، سرد، بارانی", "ckb": "گەرم، سارد، بارانی"}]}`)},
	}
	for ci := range r.lessons[cid] {
		if m, ok := meta[r.lessons[cid][ci].ID]; ok {
			r.lessons[cid][ci].CoverURL = m.cover
			r.lessons[cid][ci].Focus = m.focus
			r.lessons[cid][ci].Objective = m.obj
		}
	}
}

// ---- read side (course module) ----

func (r *MemoryCourseRepo) ListCourses(_ context.Context, language, cefr string) ([]domain.Course, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Course, 0, len(r.courses))
	for _, c := range r.courses {
		if !c.Published || (language != "" && c.Language != language) || (cefr != "" && c.CEFR != cefr) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *MemoryCourseRepo) Course(_ context.Context, id string) (*domain.Course, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.courses {
		if r.courses[i].ID == id {
			c := r.courses[i]
			return &c, nil
		}
	}
	return nil, domain.ErrCourseNotFound
}

func (r *MemoryCourseRepo) Lessons(_ context.Context, courseID string) ([]domain.Lesson, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	unitPos := map[string]int{}
	for _, u := range r.units[courseID] {
		unitPos[u.ID] = u.Position
	}
	ls := append([]domain.Lesson(nil), r.lessons[courseID]...)
	sort.Slice(ls, func(i, j int) bool {
		ui, uj := unitPos[ls[i].UnitID], unitPos[ls[j].UnitID]
		if ui != uj {
			return ui < uj
		}
		return ls[i].Position < ls[j].Position
	})
	return ls, nil
}

func (r *MemoryCourseRepo) Lesson(_ context.Context, id string) (*domain.Lesson, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ls := range r.lessons {
		for _, l := range ls {
			if l.ID == id {
				cp := l
				return &cp, nil
			}
		}
	}
	return nil, domain.ErrLessonNotFound
}

func (r *MemoryCourseRepo) Items(_ context.Context, lessonID string) ([]domain.LessonItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	its := append([]domain.LessonItem(nil), r.items[lessonID]...)
	sort.Slice(its, func(i, j int) bool { return its[i].Position < its[j].Position })
	return its, nil
}

func (r *MemoryCourseRepo) Exercises(_ context.Context, itemID string) ([]domain.Exercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]domain.Exercise(nil), r.exs[itemID]...), nil
}

func (r *MemoryCourseRepo) Exercise(_ context.Context, id string) (*domain.Exercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, list := range r.exs {
		for _, e := range list {
			if e.ID == id {
				cp := e
				return &cp, nil
			}
		}
	}
	return nil, domain.ErrExerciseNotFound
}

func (r *MemoryCourseRepo) LessonCount(_ context.Context, courseID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.lessons[courseID]), nil
}

func (r *MemoryCourseRepo) LessonCourseID(lessonID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.lessonCourse[lessonID]
	return c, ok
}

// ---- write side (admin module) ----

func (r *MemoryCourseRepo) ListAllCourses(_ context.Context) ([]domain.Course, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]domain.Course(nil), r.courses...), nil
}

func (r *MemoryCourseRepo) CreateCourse(_ context.Context, c *domain.Course) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == "" {
		c.ID = id.New()
	}
	if c.Version == 0 {
		c.Version = 1
	}
	r.courses = append(r.courses, *c)
	return nil
}

func (r *MemoryCourseRepo) SetPublished(_ context.Context, courseID string, published bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.courses {
		if r.courses[i].ID == courseID {
			r.courses[i].Published = published
			return nil
		}
	}
	return domain.ErrCourseNotFound
}

func (r *MemoryCourseRepo) MoveLessonToUnit(_ context.Context, lessonID, unitID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cid, ok := r.lessonCourse[lessonID]
	if !ok {
		return domain.ErrLessonNotFound
	}
	inCourse := false
	for _, u := range r.units[cid] {
		if u.ID == unitID {
			inCourse = true
		}
	}
	if !inCourse {
		return domain.ErrLessonNotFound
	}
	list := r.lessons[cid]
	n := 0
	for _, x := range list {
		if x.UnitID == unitID {
			n++
		}
	}
	for i := range list {
		if list[i].ID == lessonID {
			list[i].UnitID = unitID
			list[i].Position = n + 1
			return nil
		}
	}
	return domain.ErrLessonNotFound
}

func (r *MemoryCourseRepo) CreateLesson(_ context.Context, l *domain.Lesson) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.ID == "" {
		l.ID = id.New()
	}
	// resolve the owning course from the unit (units are the source of truth now)
	if l.UnitID != "" {
		for _, list := range r.units {
			for _, u := range list {
				if u.ID == l.UnitID {
					l.CourseID = u.CourseID
				}
			}
		}
	}
	if l.Position == 0 {
		n := 0
		for _, x := range r.lessons[l.CourseID] {
			if x.UnitID == l.UnitID {
				n++
			}
		}
		l.Position = n + 1
	}
	if l.Version == 0 {
		l.Version = 1
	}
	r.lessons[l.CourseID] = append(r.lessons[l.CourseID], *l)
	r.lessonCourse[l.ID] = l.CourseID
	return nil
}

func (r *MemoryCourseRepo) UpdateLesson(_ context.Context, l *domain.Lesson) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cid, ok := r.lessonCourse[l.ID]
	if !ok {
		return domain.ErrLessonNotFound
	}
	list := r.lessons[cid]
	for i := range list {
		if list[i].ID == l.ID {
			if l.Title != "" {
				list[i].Title = l.Title
			}
			list[i].CoverURL = l.CoverURL
			list[i].Focus = l.Focus
			list[i].Objective = l.Objective
			return nil
		}
	}
	return domain.ErrLessonNotFound
}

func (r *MemoryCourseRepo) MediaItems(_ context.Context, kind, language string, limit, offset int) ([]domain.LessonItem, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []domain.LessonItem
	for _, list := range r.items {
		for _, it := range list {
			if it.Kind != "podcast" && it.Kind != "video" && it.Kind != "story" {
				continue
			}
			if kind != "" && it.Kind != kind {
				continue
			}
			var p struct {
				Language   string            `json:"language"`
				Transcript []json.RawMessage `json:"transcript"`
			}
			_ = json.Unmarshal(it.Payload, &p)
			if len(p.Transcript) == 0 {
				continue
			}
			if language != "" && p.Language != language {
				continue
			}
			all = append(all, it)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return append([]domain.LessonItem(nil), all[offset:end]...), total, nil
}

func (r *MemoryCourseRepo) UpdateItem(_ context.Context, it *domain.LessonItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for lid, list := range r.items {
		for i := range list {
			if list[i].ID == it.ID {
				if len(it.Payload) > 0 {
					r.items[lid][i].Payload = it.Payload
				}
				r.items[lid][i].MediaKey = it.MediaKey
				return nil
			}
		}
	}
	return domain.ErrItemNotFound
}

func (r *MemoryCourseRepo) CreateItem(_ context.Context, it *domain.LessonItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if it.ID == "" {
		it.ID = id.New()
	}
	if it.Position == 0 {
		it.Position = len(r.items[it.LessonID]) + 1
	}
	if len(it.Payload) == 0 {
		it.Payload = raw(`{}`)
	}
	r.items[it.LessonID] = append(r.items[it.LessonID], *it)
	return nil
}

func (r *MemoryCourseRepo) CreateExercise(_ context.Context, e *domain.Exercise) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == "" {
		e.ID = id.New()
	}
	r.exs[e.LessonItemID] = append(r.exs[e.LessonItemID], *e)
	return nil
}

// MemoryProgressRepo records learning events in memory.
type MemoryProgressRepo struct {
	mu           sync.RWMutex
	entries      []domain.ProgressEntry
	lessonCourse func(lessonID string) (string, bool)
}

func NewMemoryProgressRepo(lessonCourse func(string) (string, bool)) *MemoryProgressRepo {
	return &MemoryProgressRepo{lessonCourse: lessonCourse}
}

func (r *MemoryProgressRepo) Record(_ context.Context, e domain.ProgressEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
	return nil
}

func (r *MemoryProgressRepo) ExerciseAttempts(_ context.Context, userID, exerciseID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, e := range r.entries {
		if e.UserID == userID && e.EntityType == domain.EntityExercise && e.EntityID == exerciseID {
			n++
		}
	}
	return n, nil
}

func (r *MemoryProgressRepo) CompletedLessons(_ context.Context, userID, courseID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]struct{}{}
	for _, e := range r.entries {
		if e.UserID != userID || e.EntityType != domain.EntityLesson || e.Status != domain.StatusCompleted {
			continue
		}
		if c, ok := r.lessonCourse(e.EntityID); ok && c == courseID {
			seen[e.EntityID] = struct{}{}
		}
	}
	return len(seen), nil
}

func (r *MemoryProgressRepo) CompletedLessonIDs(_ context.Context, userID, courseID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[string]struct{}{}
	out := []string{}
	for _, e := range r.entries {
		if e.UserID != userID || e.EntityType != domain.EntityLesson || e.Status != domain.StatusCompleted {
			continue
		}
		if c, ok := r.lessonCourse(e.EntityID); ok && c == courseID {
			if _, dup := seen[e.EntityID]; !dup {
				seen[e.EntityID] = struct{}{}
				out = append(out, e.EntityID)
			}
		}
	}
	return out, nil
}

func (r *MemoryCourseRepo) DeleteItem(_ context.Context, itemID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for lid, list := range r.items {
		for i, it := range list {
			if it.ID == itemID {
				r.items[lid] = append(list[:i:i], list[i+1:]...)
				delete(r.exs, itemID)
				return nil
			}
		}
	}
	return nil // idempotent
}

// ReplaceImportedItems mirrors the Postgres behaviour: drop the lesson's
// imported items, keep hand-authored ones, append the new set in order.
func (r *MemoryCourseRepo) ReplaceImportedItems(_ context.Context, lessonID string, items []domain.ImportedItem) ([]domain.LessonItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]domain.LessonItem, 0, len(r.items[lessonID]))
	base := 0
	for _, it := range r.items[lessonID] {
		if it.SourceRef != "" {
			delete(r.exs, it.ID)
			continue
		}
		kept = append(kept, it)
		if it.Position > base {
			base = it.Position
		}
	}
	out := make([]domain.LessonItem, 0, len(items))
	for i, in := range items {
		payload := in.Payload
		if len(payload) == 0 {
			payload = raw(`{}`)
		}
		it := domain.LessonItem{ID: id.New(), LessonID: lessonID, Position: base + i + 1, Kind: in.Kind, Payload: payload, SourceRef: in.SourceRef}
		for _, e := range in.Exercises {
			e.ID = id.New()
			e.LessonItemID = it.ID
			r.exs[it.ID] = append(r.exs[it.ID], e)
		}
		kept = append(kept, it)
		out = append(out, it)
	}
	r.items[lessonID] = kept
	return out, nil
}

func (r *MemoryCourseRepo) ReorderItems(_ context.Context, lessonID string, orderedIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.items[lessonID]
	byID := make(map[string]domain.LessonItem, len(list))
	for _, it := range list {
		byID[it.ID] = it
	}
	out := make([]domain.LessonItem, 0, len(list))
	for _, id := range orderedIDs {
		if it, ok := byID[id]; ok {
			it.Position = len(out) + 1
			out = append(out, it)
			delete(byID, id)
		}
	}
	// keep any items not referenced (safety), preserving original order
	for _, it := range list {
		if _, ok := byID[it.ID]; ok {
			it.Position = len(out) + 1
			out = append(out, it)
		}
	}
	r.items[lessonID] = out
	return nil
}

// ---- units (chapters) ----

func (r *MemoryCourseRepo) Units(_ context.Context, courseID string) ([]domain.Unit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	us := append([]domain.Unit(nil), r.units[courseID]...)
	sort.Slice(us, func(i, j int) bool { return us[i].Position < us[j].Position })
	return us, nil
}

func (r *MemoryCourseRepo) Unit(_ context.Context, unitID string) (*domain.Unit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, list := range r.units {
		for _, u := range list {
			if u.ID == unitID {
				cp := u
				return &cp, nil
			}
		}
	}
	return nil, domain.ErrUnitNotFound
}

func (r *MemoryCourseRepo) CreateUnit(_ context.Context, u *domain.Unit) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u.ID == "" {
		u.ID = id.New()
	}
	if u.Position == 0 {
		u.Position = len(r.units[u.CourseID]) + 1
	}
	r.units[u.CourseID] = append(r.units[u.CourseID], *u)
	return nil
}

func (r *MemoryCourseRepo) LessonsByUnit(_ context.Context, unitID string) ([]domain.Lesson, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var courseID string
	for _, list := range r.units {
		for _, u := range list {
			if u.ID == unitID {
				courseID = u.CourseID
			}
		}
	}
	var out []domain.Lesson
	for _, l := range r.lessons[courseID] {
		if l.UnitID == unitID {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

func (r *MemoryCourseRepo) ReorderUnits(_ context.Context, courseID string, orderedIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	pos := map[string]int{}
	for i, id := range orderedIDs {
		pos[id] = i + 1
	}
	for i := range r.units[courseID] {
		if p, ok := pos[r.units[courseID][i].ID]; ok {
			r.units[courseID][i].Position = p
		}
	}
	return nil
}

func (r *MemoryCourseRepo) ReorderLessons(_ context.Context, unitID string, orderedIDs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var courseID string
	for _, list := range r.units {
		for _, u := range list {
			if u.ID == unitID {
				courseID = u.CourseID
			}
		}
	}
	pos := map[string]int{}
	for i, id := range orderedIDs {
		pos[id] = i + 1
	}
	for i := range r.lessons[courseID] {
		if r.lessons[courseID][i].UnitID != unitID {
			continue
		}
		if p, ok := pos[r.lessons[courseID][i].ID]; ok {
			r.lessons[courseID][i].Position = p
		}
	}
	return nil
}

func (r *MemoryCourseRepo) SearchCourses(_ context.Context, q string, limit, offset int) ([]domain.Course, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	q = strings.ToLower(strings.TrimSpace(q))
	var filtered []domain.Course
	for _, c := range r.courses {
		if q == "" || strings.Contains(strings.ToLower(c.Title), q) {
			filtered = append(filtered, c)
		}
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return append([]domain.Course(nil), filtered[offset:end]...), total, nil
}

func arKey(t, id string) string { return t + "|" + id }

func (r *MemoryCourseRepo) GetAccessRule(_ context.Context, entityType, entityID string) (*domain.AccessRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ar, ok := r.accessRules[arKey(entityType, entityID)]; ok {
		cp := ar
		return &cp, nil
	}
	return nil, nil
}

func (r *MemoryCourseRepo) SetAccessRule(_ context.Context, rule domain.AccessRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accessRules[arKey(rule.EntityType, rule.EntityID)] = rule
	return nil
}

func (r *MemoryCourseRepo) DeleteAccessRule(_ context.Context, entityType, entityID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.accessRules, arKey(entityType, entityID))
	return nil
}

func (r *MemoryCourseRepo) ListAccessRules(_ context.Context) ([]domain.AccessRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.AccessRule, 0, len(r.accessRules))
	for _, ar := range r.accessRules {
		out = append(out, ar)
	}
	return out, nil
}
