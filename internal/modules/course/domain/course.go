// Package domain holds the course bounded context: the learning tree
// (course -> lesson -> item -> exercise) and progress. Order is ALWAYS taken
// from the Position field — never hardcoded (a hard spec constraint).
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/leksuu/platform/internal/shared/apperr"
	"strings"
	"time"
)

// CEFR levels and item/exercise kinds mirror the database CHECK constraints, so
// the vocabulary lives in one place instead of as scattered string literals.
const (
	MediaBucketTitle = "__media_library__"
	KindText         = "text"
	KindImage        = "image"
	KindAudio        = "audio"
	KindVideo        = "video"
	KindMusic        = "music"
	KindExercise     = "exercise"
	KindSpeaking     = "speaking"
	KindDialogue     = "dialogue"
	KindMiniStory    = "mini_story"
	KindPOVStory     = "pov_story"
	KindStory        = "story"
	KindPodcast      = "podcast"
	KindGrammar      = "grammar"
	KindReading      = "reading"  // a longer text read before its comprehension questions
	KindWriting      = "writing"  // open writing task checked by the AI writing coach
	KindRoleplay     = "roleplay" // scenario conversation with the AI tutor
)

// Course is a CEFR-leveled program in a target language.
type Course struct {
	ID          string
	Language    string
	CEFR        string
	Title       string
	Description string
	ImageURL    string
	Version     int
	Published   bool
	SourceID    string // stable external ID of an imported course pack; empty when hand-authored
}

// Lesson is an ordered unit within a course (order = Position).
// Unit (a.k.a. chapter) groups lessons inside a course. Order = Position.
type Unit struct {
	ID          string
	CourseID    string
	Position    int
	Title       string
	Description string
	SourceID    string // stable external chapter ID (e.g. a Busuu chapter id); empty when hand-authored
}

type Lesson struct {
	ID        string
	SourceID  string // stable external ID used to make course imports idempotent
	CourseID  string
	UnitID    string
	Position  int
	Title     string
	Version   int
	CoverURL  string          // optional lesson cover image (data-URI or URL)
	Focus     string          // vocabulary | grammar | pronunciation | phrases | conversation
	Objective json.RawMessage // {"summary":{fa,ckb},"points":[{fa,ckb}]} — Busuu-style intro
	Kind      string          // lesson | checkpoint (end-of-chapter test)
}

// KindOrDefault reports a lesson kind, treating an unset value as a lesson.
func KindOrDefault(k string) string {
	if k == LessonKindCheckpoint {
		return k
	}
	return LessonKindLesson
}

// Target languages a course can teach (all Latin script). DefaultLanguage is
// used when a course has none recorded.
const DefaultLanguage = "en"

// SupportedLanguages are the initial target languages (the migration and the
// in-memory store seed them); the live list is the admin-managed languages table.
var SupportedLanguages = []string{"en", "de", "fr", "tr"}

// Language is a target language the platform can teach. Disabled languages
// keep their courses but are hidden from learners.
type Language struct {
	Code     string // ISO 639 code, 2–3 lowercase letters (en, de, fr, tr…)
	Enabled  bool
	Position int
}

// Language errors.
var (
	ErrLanguageCode     = apperr.New("language.invalid_code", apperr.KindValidation)
	ErrLanguageExists   = apperr.New("language.exists", apperr.KindConflict)
	ErrLanguageNotFound = apperr.New("language.not_found", apperr.KindNotFound)
	// ErrLearnerOnly: the learning app is for learner accounts (teachers and
	// admins have their own workspaces; admins may only preview).
	ErrLearnerOnly = apperr.New("auth.learner_only", apperr.KindForbidden)
)

// Lesson kinds.
const (
	LessonKindLesson     = "lesson"
	LessonKindCheckpoint = "checkpoint"
)

// LessonItem is an ordered piece of a lesson (order = Position). Payload carries
// type-specific content; RefID points at a typed content row when relevant.
type LessonItem struct {
	ID       string
	LessonID string
	Position int
	Kind     string
	RefID    *string
	Payload  json.RawMessage
	MediaKey *string
	// SourceRef is set on items created by a course import (the external node
	// ID). A re-import replaces only these; hand-authored items keep it empty.
	SourceRef string
}

// ImportedItem is one lesson item produced by a course import, together with
// its exercises. A lesson's imported items are always replaced as one set.
type ImportedItem struct {
	Kind      string
	Payload   json.RawMessage
	SourceRef string
	Exercises []Exercise
}

// ImportedItemsResult reports how a lesson's imported items were synced. Items
// are matched by (SourceRef, occurrence) so a re-import keeps their IDs (and the
// learner data hanging off them) instead of deleting and recreating them.
type ImportedItemsResult struct {
	Items     []LessonItem
	Created   int
	Updated   int
	Unchanged int
	Removed   int
}

// ImportSource records an immutable source document used by an admin import.
// Keeping the original checksum and metadata makes imports auditable without
// mixing source provenance into the learner-facing course hierarchy.
type ImportSource struct {
	ID           string          `json:"id"`
	FileName     string          `json:"file_name"`
	StorageKey   string          `json:"storage_key"`
	URL          string          `json:"url"`
	ContentType  string          `json:"content_type"`
	ContentBytes int             `json:"content_bytes"`
	SHA256       string          `json:"sha256"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ContentTranslation is a translation of one imported source string (a grammar
// explanation, a table label) into a learner locale. It is keyed by a hash of
// the exact source text and lives outside the lesson payload, so the source is
// never overwritten and re-imports keep the translation.
type ContentTranslation struct {
	SourceHash string
	Locale     string
	SourceLang string
	SourceText string
	Text       string
	Origin     string // human | machine | source
}

// Translation origins.
const (
	OriginHuman   = "human"
	OriginMachine = "machine"
	OriginSource  = "source"
)

// Exercise is an interactive task attached to a lesson item. AnswerKey is the
// solution and must never be exposed to learners by the delivery layer.
type Exercise struct {
	ID           string
	LessonItemID string
	Type         string
	Prompt       json.RawMessage
	AnswerKey    json.RawMessage
	Difficulty   int
}

// Access requirements for course content. A course/unit/lesson can require a
// specific subscription product or be free. No rule means "inherit" from the
// parent (lesson -> unit -> course -> default).
const (
	AccessFree    = "free"
	AccessCourses = "courses"
	AccessClasses = "classes"
	AccessDefault = AccessCourses // root default preserves the premium model
)

// AccessRule pins an access requirement to one entity.
type AccessRule struct {
	EntityType  string `json:"entity_type"` // course | unit | lesson
	EntityID    string `json:"entity_id"`
	Requirement string `json:"requirement"` // free | courses | classes
}

// SourceHash is the content_translations key of a source string.
func SourceHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}
