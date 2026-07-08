// Package domain holds the course bounded context: the learning tree
// (course -> lesson -> item -> exercise) and progress. Order is ALWAYS taken
// from the Position field — never hardcoded (a hard spec constraint).
package domain

import "encoding/json"

// CEFR levels and item/exercise kinds mirror the database CHECK constraints, so
// the vocabulary lives in one place instead of as scattered string literals.
const (
	KindText      = "text"
	KindImage     = "image"
	KindAudio     = "audio"
	KindVideo     = "video"
	KindExercise  = "exercise"
	KindSpeaking  = "speaking"
	KindDialogue  = "dialogue"
	KindMiniStory = "mini_story"
	KindPOVStory  = "pov_story"
	KindStory     = "story"
	KindPodcast   = "podcast"
	KindGrammar   = "grammar"
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
}

// Lesson is an ordered unit within a course (order = Position).
// Unit (a.k.a. chapter) groups lessons inside a course. Order = Position.
type Unit struct {
	ID          string
	CourseID    string
	Position    int
	Title       string
	Description string
}

type Lesson struct {
	ID        string
	CourseID  string
	UnitID    string
	Position  int
	Title     string
	Version   int
	CoverURL  string          // optional lesson cover image (data-URI or URL)
	Focus     string          // vocabulary | grammar | pronunciation | phrases | conversation
	Objective json.RawMessage // {"summary":{fa,ckb},"points":[{fa,ckb}]} — Busuu-style intro
}

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
}

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
