// Package usecase holds content-authoring rules for the admin module. It reuses
// the course bounded context's entities and writes through a Repository the
// content store satisfies (the same store the course module reads from).
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"

	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	mediadomain "github.com/leksuu/platform/internal/modules/media/domain"
	srsdomain "github.com/leksuu/platform/internal/modules/srs/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
)

// Repository is the content write+read surface authoring needs (satisfied by the
// shared content store / Postgres repo).
type Repository interface {
	Languages(ctx context.Context) ([]coursedomain.Language, error)
	SaveLanguage(ctx context.Context, l coursedomain.Language) error
	ListAllCourses(ctx context.Context) ([]coursedomain.Course, error)
	CreateCourse(ctx context.Context, c *coursedomain.Course) error
	Course(ctx context.Context, id string) (*coursedomain.Course, error)
	SetPublished(ctx context.Context, id string, published bool) error
	CreateLesson(ctx context.Context, l *coursedomain.Lesson) error
	UpdateLesson(ctx context.Context, l *coursedomain.Lesson) error
	MoveLessonToUnit(ctx context.Context, lessonID, unitID string) error
	Lesson(ctx context.Context, id string) (*coursedomain.Lesson, error)
	Lessons(ctx context.Context, courseID string) ([]coursedomain.Lesson, error)
	CreateItem(ctx context.Context, it *coursedomain.LessonItem) error
	UpdateItem(ctx context.Context, it *coursedomain.LessonItem) error
	UpdateItemPayloadIfUnchanged(ctx context.Context, itemID string, expected, replacement json.RawMessage) (bool, error)
	CreateExercise(ctx context.Context, e *coursedomain.Exercise) error
	Items(ctx context.Context, lessonID string) ([]coursedomain.LessonItem, error)
	DeleteItem(ctx context.Context, itemID string) error
	ReplaceImportedItems(ctx context.Context, lessonID string, items []coursedomain.ImportedItem) (*coursedomain.ImportedItemsResult, error)
	LessonsBySourceID(ctx context.Context, courseID, sourceID string) ([]coursedomain.Lesson, error)
	SetUnitSourceID(ctx context.Context, unitID, sourceID string) error
	ContentTranslations(ctx context.Context, hashes []string) ([]coursedomain.ContentTranslation, error)
	UpsertContentTranslations(ctx context.Context, ts []coursedomain.ContentTranslation) error
	DeleteContentTranslation(ctx context.Context, sourceHash, locale string) error
	ReorderItems(ctx context.Context, lessonID string, orderedIDs []string) error
	Units(ctx context.Context, courseID string) ([]coursedomain.Unit, error)
	Unit(ctx context.Context, id string) (*coursedomain.Unit, error)
	CreateUnit(ctx context.Context, u *coursedomain.Unit) error
	LessonsByUnit(ctx context.Context, unitID string) ([]coursedomain.Lesson, error)
	ReorderUnits(ctx context.Context, courseID string, orderedIDs []string) error
	ReorderLessons(ctx context.Context, unitID string, orderedIDs []string) error
	SearchCourses(ctx context.Context, q string, limit, offset int) ([]coursedomain.Course, int, error)
	SetAccessRule(ctx context.Context, rule coursedomain.AccessRule) error
	DeleteAccessRule(ctx context.Context, entityType, entityID string) error
	ListAccessRules(ctx context.Context) ([]coursedomain.AccessRule, error)
	SaveImportSource(ctx context.Context, src *coursedomain.ImportSource) error
	ListImportSources(ctx context.Context, limit, offset int) ([]coursedomain.ImportSource, int, error)
}

// VocabStore is the admin-facing slice of the vocabulary bank (vocabulary_items),
// satisfied structurally by the SRS repository.
type VocabStore interface {
	ListVocab(ctx context.Context, language, q string, limit, offset int) ([]srsdomain.Vocab, int, error)
	LookupVocab(ctx context.Context, language, lemma string) (*srsdomain.Vocab, error)
	GetVocab(ctx context.Context, id string) (*srsdomain.Vocab, error)
	EnsureVocab(ctx context.Context, v *srsdomain.Vocab) error
	UpdateVocab(ctx context.Context, v *srsdomain.Vocab) error
	DeleteVocab(ctx context.Context, id string) error
}

type Service struct {
	repo             Repository
	vocab            VocabStore
	store            mediadomain.Store
	mediaJobs        MediaJobStore
	timingExceptions TimingExceptionStore
	bulkMaxBytes     int64
}

func NewService(repo Repository, vocab VocabStore, stores ...mediadomain.Store) *Service {
	var store mediadomain.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	return &Service{repo: repo, vocab: vocab, store: store, bulkMaxBytes: 512 << 20}
}

// SetBulkMaxBytes sets the request limit for story ZIP imports. It is separate
// from the per-file media limit because a collection ZIP can contain many MP3s.
func (s *Service) SetBulkMaxBytes(n int64) {
	if n > 0 {
		s.bulkMaxBytes = n
	}
}
func (s *Service) BulkMaxBytes() int64 { return s.bulkMaxBytes }

func (s *Service) ListCourses(ctx context.Context) ([]coursedomain.Course, error) {
	return s.repo.ListAllCourses(ctx)
}

type CreateCourseInput struct{ Language, CEFR, Title, Description, ImageURL, SourceID string }

// CreateCourse authors a new course in any target language (unpublished). With
// a SourceID (an imported course pack) it is idempotent: the course imported
// from that source earlier is returned with reused=true, so retrying an
// interrupted "new course" import never creates a second course.
func (s *Service) CreateCourse(ctx context.Context, in CreateCourseInput) (*coursedomain.Course, bool, error) {
	if in.Language == "" || in.Title == "" || !s.languageKnown(ctx, in.Language) {
		return nil, false, apperr.ErrBadRequest
	}
	sourceID := strings.TrimSpace(in.SourceID)
	if sourceID != "" {
		courses, err := s.repo.ListAllCourses(ctx)
		if err != nil {
			return nil, false, err
		}
		for i := range courses {
			if courses[i].SourceID == sourceID {
				return &courses[i], true, nil
			}
		}
	}
	cefr := in.CEFR
	if cefr == "" {
		cefr = "A1"
	}
	c := &coursedomain.Course{Language: in.Language, CEFR: cefr, Title: in.Title, Description: in.Description, ImageURL: in.ImageURL, Published: false, SourceID: sourceID}
	if err := s.repo.CreateCourse(ctx, c); err != nil {
		return nil, false, err
	}
	return c, false, nil
}

func (s *Service) Publish(ctx context.Context, courseID string, published bool) error {
	if _, err := s.repo.Course(ctx, courseID); err != nil {
		return err
	}
	return s.repo.SetPublished(ctx, courseID, published)
}

func (s *Service) Lessons(ctx context.Context, courseID string) ([]coursedomain.Lesson, error) {
	return s.repo.Lessons(ctx, courseID)
}

func (s *Service) Items(ctx context.Context, lessonID string) ([]coursedomain.LessonItem, error) {
	return s.repo.Items(ctx, lessonID)
}

func (s *Service) DeleteItem(ctx context.Context, itemID string) error {
	return s.repo.DeleteItem(ctx, itemID)
}

func (s *Service) ReorderItems(ctx context.Context, lessonID string, orderedIDs []string) error {
	return s.repo.ReorderItems(ctx, lessonID, orderedIDs)
}

func (s *Service) SearchCourses(ctx context.Context, q string, limit, offset int) ([]coursedomain.Course, int, error) {
	return s.repo.SearchCourses(ctx, q, limit, offset)
}

func (s *Service) Units(ctx context.Context, courseID string) ([]coursedomain.Unit, error) {
	return s.repo.Units(ctx, courseID)
}

// CreateUnit adds a unit (chapter) at the end of a course. With a sourceID (an
// imported chapter) it is idempotent: the unit already imported from that
// chapter is returned with reused=true. A unit an older importer created
// without a source id but with the same title is adopted (its source id is
// recorded) instead of being duplicated.
func (s *Service) CreateUnit(ctx context.Context, courseID, title, description, sourceID string) (*coursedomain.Unit, bool, error) {
	if title == "" {
		return nil, false, apperr.ErrBadRequest
	}
	if _, err := s.repo.Course(ctx, courseID); err != nil {
		return nil, false, err
	}
	sourceID = strings.TrimSpace(sourceID)
	if sourceID != "" {
		units, err := s.repo.Units(ctx, courseID)
		if err != nil {
			return nil, false, err
		}
		for i := range units {
			if units[i].SourceID == sourceID {
				return &units[i], true, nil
			}
		}
		for i := range units {
			if units[i].SourceID == "" && normalizeTitle(units[i].Title) == normalizeTitle(title) {
				if err := s.repo.SetUnitSourceID(ctx, units[i].ID, sourceID); err != nil {
					return nil, false, err
				}
				units[i].SourceID = sourceID
				return &units[i], true, nil
			}
		}
	}
	u := &coursedomain.Unit{CourseID: courseID, Title: title, Description: description, SourceID: sourceID}
	if err := s.repo.CreateUnit(ctx, u); err != nil {
		return nil, false, err
	}
	return u, false, nil
}

func normalizeTitle(v string) string {
	return strings.Join(strings.Fields(strings.ToLower(v)), " ")
}

func (s *Service) ReorderUnits(ctx context.Context, courseID string, orderedIDs []string) error {
	return s.repo.ReorderUnits(ctx, courseID, orderedIDs)
}

func (s *Service) LessonsByUnit(ctx context.Context, unitID string) ([]coursedomain.Lesson, error) {
	return s.repo.LessonsByUnit(ctx, unitID)
}

func (s *Service) ReorderLessons(ctx context.Context, unitID string, orderedIDs []string) error {
	return s.repo.ReorderLessons(ctx, unitID, orderedIDs)
}

// LessonPlacement says how AddLessonToUnit satisfied an imported lesson.
type LessonPlacement struct {
	Reused bool // an existing lesson with the source id was used
	Moved  bool // ...and it was moved here from another unit of the course
	// Duplicates are other lessons of the course carrying the same source id
	// (left by an older importer). They are reported, never deleted: they may
	// hold learner progress.
	Duplicates []string
}

// AddLessonToUnit creates a lesson in a unit. With a sourceID (an imported
// lesson) it is idempotent across the whole course: an existing lesson with
// that source id is reused, and moved into this unit if an earlier import put
// it elsewhere — never duplicated.
func (s *Service) AddLessonToUnit(ctx context.Context, unitID, title, sourceID string, position int) (*coursedomain.Lesson, LessonPlacement, error) {
	var placed LessonPlacement
	if title == "" {
		return nil, placed, apperr.ErrBadRequest
	}
	unit, err := s.repo.Unit(ctx, unitID)
	if err != nil {
		return nil, placed, err
	}
	sourceID = strings.TrimSpace(sourceID)
	if sourceID != "" {
		found, err := s.repo.LessonsBySourceID(ctx, unit.CourseID, sourceID)
		if err != nil {
			return nil, placed, err
		}
		if len(found) > 0 {
			// A copy already in this unit is used as is: a unit holds at most
			// one lesson per source (lessons_unit_source_id_uq), so no other
			// copy could be moved in. Otherwise move the copy holding the most
			// content (imported and hand-authored items, and with them learner
			// progress): an empty shell left by an older importer must never
			// win over the real lesson.
			pick := -1
			for i := range found {
				if found[i].UnitID == unitID {
					pick = i
					break
				}
			}
			if pick < 0 {
				best := -1
				for i := range found {
					items, err := s.repo.Items(ctx, found[i].ID)
					if err != nil {
						return nil, placed, err
					}
					if len(items) > best {
						pick, best = i, len(items)
					}
				}
			}
			for i := range found {
				if i != pick {
					placed.Duplicates = append(placed.Duplicates, found[i].ID)
				}
			}
			l := found[pick]
			placed.Reused = true
			if l.UnitID != unitID {
				if err := s.repo.MoveLessonToUnit(ctx, l.ID, unitID); err != nil {
					return nil, placed, err
				}
				placed.Moved = true
				moved, err := s.repo.Lesson(ctx, l.ID)
				if err != nil {
					return nil, placed, err
				}
				l = *moved
			}
			return &l, placed, nil
		}
	}
	l := &coursedomain.Lesson{UnitID: unitID, Title: title, SourceID: sourceID, Position: position}
	if err := s.repo.CreateLesson(ctx, l); err != nil {
		return nil, placed, err
	}
	return l, placed, nil
}

func (s *Service) UpdateLesson(ctx context.Context, id, title, focus, coverURL string, objective json.RawMessage, kind string) (*coursedomain.Lesson, error) {
	if kind != "" && kind != coursedomain.LessonKindLesson && kind != coursedomain.LessonKindCheckpoint {
		return nil, apperr.ErrBadRequest
	}
	l, err := s.repo.Lesson(ctx, id)
	if err != nil {
		return nil, err
	}
	if title != "" {
		l.Title = title
	}
	l.Focus = focus
	l.CoverURL = coverURL
	if len(objective) > 0 {
		l.Objective = objective
	}
	if kind != "" {
		l.Kind = kind
	}
	if err := s.repo.UpdateLesson(ctx, l); err != nil {
		return nil, err
	}
	return l, nil
}

// MoveLessonToUnit moves a lesson into another unit of its course (appended at
// the end). Used by re-imports when a lesson's unit grouping changed.
func (s *Service) MoveLessonToUnit(ctx context.Context, lessonID, unitID string) error {
	if lessonID == "" || unitID == "" {
		return apperr.ErrBadRequest
	}
	l, err := s.repo.Lesson(ctx, lessonID)
	if err != nil {
		return err
	}
	u, err := s.repo.Unit(ctx, unitID)
	if err != nil {
		return err
	}
	if u.CourseID != l.CourseID {
		return apperr.ErrBadRequest
	}
	if l.UnitID == unitID {
		return nil
	}
	return s.repo.MoveLessonToUnit(ctx, lessonID, unitID)
}

func (s *Service) AddLesson(ctx context.Context, courseID, title string, position int) (*coursedomain.Lesson, error) {
	if title == "" {
		return nil, apperr.ErrBadRequest
	}
	if _, err := s.repo.Course(ctx, courseID); err != nil {
		return nil, err
	}
	l := &coursedomain.Lesson{CourseID: courseID, Title: title, Position: position}
	if err := s.repo.CreateLesson(ctx, l); err != nil {
		return nil, err
	}
	return l, nil
}

func (s *Service) AddItem(ctx context.Context, lessonID, kind string, payload json.RawMessage, position int, sourceRef string) (*coursedomain.LessonItem, error) {
	if kind == "" {
		return nil, apperr.ErrBadRequest
	}
	it := &coursedomain.LessonItem{LessonID: lessonID, Kind: kind, Payload: payload, Position: position, SourceRef: strings.TrimSpace(sourceRef)}
	if err := s.repo.CreateItem(ctx, it); err != nil {
		return nil, err
	}
	return it, nil
}

func (s *Service) UpdateItem(ctx context.Context, itemID string, payload json.RawMessage, mediaKey *string) (*coursedomain.LessonItem, error) {
	if itemID == "" {
		return nil, apperr.ErrBadRequest
	}
	if current, err := s.findMediaItem(ctx, itemID); err == nil {
		var original struct {
			SourceID string `json:"source_id"`
		}
		_ = json.Unmarshal(current.Payload, &original)
		if original.SourceID != "" {
			if err := guardImportedMutation(current.Payload, payload); err != nil {
				return nil, err
			}
			updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, itemID, current.Payload, payload)
			if err != nil {
				return nil, err
			}
			if !updated {
				return nil, ErrMediaJobConflict
			}
			current.Payload = payload
			return current, nil
		}
	} else if !errors.Is(err, coursedomain.ErrItemNotFound) {
		return nil, err
	}
	it := &coursedomain.LessonItem{ID: itemID, Payload: payload, MediaKey: mediaKey}
	if err := s.repo.UpdateItem(ctx, it); err != nil {
		return nil, err
	}
	return it, nil
}

func (s *Service) AddExercise(ctx context.Context, itemID, typ string, prompt, answerKey json.RawMessage, difficulty int) (*coursedomain.Exercise, error) {
	if typ == "" {
		return nil, apperr.ErrBadRequest
	}
	e := &coursedomain.Exercise{LessonItemID: itemID, Type: typ, Prompt: prompt, AnswerKey: answerKey, Difficulty: difficulty}
	if err := s.repo.CreateExercise(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// maxImportedItems bounds one lesson's import payload (the largest Busuu A1
// lesson has ~60 nodes).
const maxImportedItems = 500

// ReplaceImportedItems atomically replaces what earlier imports put in a
// lesson. Every item needs a kind and a source_ref (that is what marks it as
// imported), and every exercise a type; the DB CHECKs validate the values.
func (s *Service) ReplaceImportedItems(ctx context.Context, lessonID string, items []coursedomain.ImportedItem) (*coursedomain.ImportedItemsResult, error) {
	if lessonID == "" || len(items) > maxImportedItems {
		return nil, apperr.ErrBadRequest
	}
	if _, err := s.repo.Lesson(ctx, lessonID); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Kind = strings.TrimSpace(items[i].Kind)
		items[i].SourceRef = strings.TrimSpace(items[i].SourceRef)
		if items[i].Kind == "" || items[i].SourceRef == "" {
			return nil, apperr.ErrBadRequest
		}
		for _, e := range items[i].Exercises {
			if e.Type == "" {
				return nil, apperr.ErrBadRequest
			}
		}
	}
	return s.repo.ReplaceImportedItems(ctx, lessonID, items)
}

func (s *Service) LessonTemplates() []LessonTemplate { return lessonTemplates() }

// LessonContextResult is a lesson plus its unit and course summaries, used by the
// authoring UI for breadcrumbs and level-aware lesson templates.
type LessonContextResult struct {
	Lesson coursedomain.Lesson
	Unit   *coursedomain.Unit
	Course *coursedomain.Course
}

func (s *Service) LessonContextOf(ctx context.Context, lessonID string) (*LessonContextResult, error) {
	l, err := s.repo.Lesson(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	res := &LessonContextResult{Lesson: *l}
	if l.UnitID != "" {
		res.Unit, _ = s.repo.Unit(ctx, l.UnitID)
	}
	if l.CourseID != "" {
		res.Course, _ = s.repo.Course(ctx, l.CourseID)
	}
	return res, nil
}

func (s *Service) SearchVocab(ctx context.Context, language, q string, limit, offset int) ([]srsdomain.Vocab, int, error) {
	return s.vocab.ListVocab(ctx, language, q, limit, offset)
}

func (s *Service) CreateVocab(ctx context.Context, language, kind, lemma string, meaning map[string]string, example, pronunciation string) (*srsdomain.Vocab, error) {
	lemma = strings.ToLower(strings.TrimSpace(lemma))
	if language == "" || lemma == "" {
		return nil, apperr.ErrBadRequest
	}
	if kind == "" {
		kind = "word"
	}
	v := &srsdomain.Vocab{Language: language, Kind: kind, Lemma: lemma, Meaning: meaning, Example: example, Pronunciation: pronunciation}
	if err := s.vocab.EnsureVocab(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Service) UpdateVocab(ctx context.Context, id string, meaning map[string]string, example, pronunciation string) (*srsdomain.Vocab, error) {
	existing, err := s.vocab.GetVocab(ctx, id)
	if err != nil {
		return nil, err
	}
	existing.Meaning = meaning
	existing.Example = example
	existing.Pronunciation = pronunciation
	if err := s.vocab.UpdateVocab(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *Service) DeleteVocab(ctx context.Context, id string) error {
	return s.vocab.DeleteVocab(ctx, id)
}

// SetAccess pins or clears the access requirement of a course/unit/lesson.
func (s *Service) SetAccess(ctx context.Context, entityType, entityID, requirement string) error {
	switch entityType {
	case "course", "unit", "lesson":
	default:
		return apperr.ErrBadRequest
	}
	if requirement == "" || requirement == "inherit" {
		return s.repo.DeleteAccessRule(ctx, entityType, entityID)
	}
	if requirement != coursedomain.AccessFree && requirement != coursedomain.AccessCourses && requirement != coursedomain.AccessClasses {
		return apperr.ErrBadRequest
	}
	return s.repo.SetAccessRule(ctx, coursedomain.AccessRule{EntityType: entityType, EntityID: entityID, Requirement: requirement})
}

// AccessRules lists all explicit access rules for the admin console.
func (s *Service) AccessRules(ctx context.Context) ([]coursedomain.AccessRule, error) {
	return s.repo.ListAccessRules(ctx)
}

// MediaBucketTitle marks the hidden system course that hosts standalone media
// library items (films/podcasts) so admins never navigate the course tree to
// publish one. It stays unpublished, so it never appears in the learner catalogue.
const MediaBucketTitle = coursedomain.MediaBucketTitle

// MediaCue is one transcript line. Start/End are optional: untimed lines are
// valid for podcasts, and films can be timed later in the lesson editor.
// Translation maps a UI locale ("fa"/"ckb") to the line's translation.
type MediaCue struct {
	Start        *float64          `json:"start,omitempty"`
	End          *float64          `json:"end,omitempty"`
	Text         string            `json:"text"`
	Translation  map[string]string `json:"translation,omitempty"`
	TimingStatus string            `json:"timing_status,omitempty"`
}

// ImportMediaInput stages standalone media as a draft.
type ImportMediaInput struct {
	Kind        string
	Title       string
	Language    string
	CEFR        string
	URL         string
	Cover       string
	Transcript  []MediaCue
	SourceID    string
	SourceURL   string
	Attribution json.RawMessage
	License     json.RawMessage
}

// ImportMedia stages standalone media in the hidden system bucket. No course-tree
// navigation needed; the item reuses the LessonItem machinery (transcript,
// shadowing, clip-replay) so nothing downstream changes.
func (s *Service) ImportMedia(ctx context.Context, in ImportMediaInput) (*coursedomain.LessonItem, error) {
	kind := strings.TrimSpace(in.Kind)
	if kind != coursedomain.KindVideo && kind != coursedomain.KindPodcast && kind != coursedomain.KindStory {
		return nil, apperr.ErrBadRequest
	}
	// Stories are text-first graded readers: no media URL required.
	if strings.TrimSpace(in.Title) == "" || (len(in.Transcript) == 0 && strings.TrimSpace(in.URL) == "") ||
		(kind != coursedomain.KindStory && strings.TrimSpace(in.URL) == "") {
		return nil, apperr.ErrBadRequest
	}
	lessonID, err := s.ensureMediaLesson(ctx)
	if err != nil {
		return nil, err
	}
	lang := strings.TrimSpace(in.Language)
	if lang == "" {
		lang = "en"
	}
	payload, err := json.Marshal(map[string]any{
		"title":       strings.TrimSpace(in.Title),
		"language":    lang,
		"level":       strings.TrimSpace(in.CEFR),
		"url":         strings.TrimSpace(in.URL),
		"cover":       strings.TrimSpace(in.Cover),
		"transcript":  in.Transcript,
		"source_id":   strings.TrimSpace(in.SourceID),
		"source_url":  strings.TrimSpace(in.SourceURL),
		"attribution": rawOrNil(in.Attribution),
		"license":     rawOrNil(in.License),
		"published":   false,
	})
	if err != nil {
		return nil, err
	}
	// Position zero asks the repository to append after the highest existing
	// position. Counting rows collides when a prior item was deleted or when
	// positions begin at one, as they do in PostgreSQL.
	return s.AddItem(ctx, lessonID, kind, payload, 0, "")
}

// UpdateMedia rewrites an existing media item's payload (title, language, level,
// url, transcript) in place — used by the standalone media editor. Kind is not
// changed here (UpdateItem persists payload only).
func (s *Service) UpdateMedia(ctx context.Context, itemID string, in ImportMediaInput) (*coursedomain.LessonItem, error) {
	if strings.TrimSpace(itemID) == "" {
		return nil, apperr.ErrBadRequest
	}
	// URL is optional here: stories have none, and kind can't change in this
	// editor. Per-kind strictness (video/podcast must have a URL) is enforced
	// at creation and mirrored in the importer UI; this endpoint is admin-only.
	if strings.TrimSpace(in.Title) == "" || (len(in.Transcript) == 0 && strings.TrimSpace(in.URL) == "") {
		return nil, apperr.ErrBadRequest
	}
	lang := strings.TrimSpace(in.Language)
	if lang == "" {
		lang = "en"
	}
	existing, err := s.findMediaItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	payloadMap := map[string]any{}
	_ = json.Unmarshal(existing.Payload, &payloadMap)
	// Imported source text, audio and accepted timing are immutable through the
	// general editor. Purpose-built CAS endpoints handle timing/translation.
	if sourceID, _ := payloadMap["source_id"].(string); sourceID != "" {
		if sha, _ := payloadMap["audio_sha256"].(string); sha != "" {
			oldURL, _ := payloadMap["url"].(string)
			oldCues, _ := payloadMap["transcript"].([]any)
			if oldURL != strings.TrimSpace(in.URL) || len(oldCues) != len(in.Transcript) {
				return nil, ErrStoryTimingConflict
			}
			for i, raw := range oldCues {
				old, ok := raw.(map[string]any)
				if !ok || old["text"] != in.Transcript[i].Text {
					return nil, ErrStoryTimingConflict
				}
				start, hasStart := old["start"].(float64)
				end, hasEnd := old["end"].(float64)
				if hasStart != hasEnd || (hasStart && (in.Transcript[i].Start == nil || in.Transcript[i].End == nil || *in.Transcript[i].Start != start || *in.Transcript[i].End != end)) {
					return nil, ErrStoryTimingConflict
				}
				if !hasStart && (in.Transcript[i].Start != nil || in.Transcript[i].End != nil) {
					return nil, ErrStoryTimingConflict
				}
				statuses, _ := payloadMap["translation_status"].(map[string]any)
				for _, lang := range []string{"fa", "ckb"} {
					if statuses[lang] == "approved" || statuses[lang] == "human_approved" {
						oldTranslations, _ := old["translation"].(map[string]any)
						if value, _ := oldTranslations[lang].(string); value != in.Transcript[i].Translation[lang] {
							return nil, ErrStoryTranslationConflict
						}
					}
				}
			}
		}
	}
	payloadMap["title"] = strings.TrimSpace(in.Title)
	payloadMap["language"] = lang
	payloadMap["level"] = strings.TrimSpace(in.CEFR)
	payloadMap["url"] = strings.TrimSpace(in.URL)
	newCover := strings.TrimSpace(in.Cover)
	oldCover, _ := payloadMap["cover"].(string)
	if newCover != oldCover {
		if oldCover != "" {
			versions, _ := payloadMap["cover_versions"].([]any)
			payloadMap["cover_versions"] = append(versions, oldCover)
		}
		payloadMap["cover_source"] = "manual"
		delete(payloadMap, "cover_key")
		delete(payloadMap, "cover_asset")
	}
	payloadMap["cover"] = newCover
	// An imported transcript carries timing, translation reviews and editorial
	// history beyond the general editor's fields. Keep it intact here; targeted
	// compare-and-swap endpoints own changes to those cues.
	if sourceID, _ := payloadMap["source_id"].(string); sourceID == "" {
		payloadMap["transcript"] = in.Transcript
	}
	if strings.TrimSpace(in.SourceID) != "" {
		payloadMap["source_id"] = strings.TrimSpace(in.SourceID)
	}
	if strings.TrimSpace(in.SourceURL) != "" {
		payloadMap["source_url"] = strings.TrimSpace(in.SourceURL)
	}
	if len(in.Attribution) > 0 {
		payloadMap["attribution"] = rawOrNil(in.Attribution)
	}
	if len(in.License) > 0 {
		payloadMap["license"] = rawOrNil(in.License)
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, itemID, existing.Payload, payload)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrMediaJobConflict
	}
	return &coursedomain.LessonItem{ID: itemID, Kind: existing.Kind, LessonID: existing.LessonID, Payload: payload, MediaKey: existing.MediaKey}, nil
}

func (s *Service) findMediaItem(ctx context.Context, itemID string) (*coursedomain.LessonItem, error) {
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
				for i := range items {
					if items[i].ID == itemID {
						return &items[i], nil
					}
				}
			}
		}
	}
	return nil, coursedomain.ErrItemNotFound
}

// AdminMedia lists both staged drafts and published media from the existing
// hidden media bucket. Learner media queries deliberately exclude drafts.
func (s *Service) AdminMedia(ctx context.Context, kind string) ([]coursedomain.LessonItem, error) {
	bySource, err := existingStories(ctx, s)
	if err != nil {
		return nil, err
	}
	items := make([]coursedomain.LessonItem, 0, len(bySource))
	for _, it := range bySource {
		if kind == "" || it.Kind == kind {
			items = append(items, it)
		}
	}
	// Non-story media does not have source IDs, so collect it separately.
	courses, err := s.repo.ListAllCourses(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.ID] = true
	}
	for _, c := range courses {
		if c.Title == MediaBucketTitle {
			units, e := s.repo.Units(ctx, c.ID)
			if e != nil {
				return nil, e
			}
			for _, u := range units {
				lessons, e := s.repo.LessonsByUnit(ctx, u.ID)
				if e != nil {
					return nil, e
				}
				for _, l := range lessons {
					all, e := s.repo.Items(ctx, l.ID)
					if e != nil {
						return nil, e
					}
					for _, it := range all {
						if !seen[it.ID] && (kind == "" || it.Kind == kind) && (it.Kind == coursedomain.KindStory || it.Kind == coursedomain.KindPodcast || it.Kind == coursedomain.KindVideo) {
							items = append(items, it)
							seen[it.ID] = true
						}
					}
				}
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Position < items[j].Position })
	return items, nil
}

func (s *Service) SetMediaPublished(ctx context.Context, itemID string, published bool) (*coursedomain.LessonItem, error) {
	it, err := s.findMediaItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if json.Unmarshal(it.Payload, &payload) != nil {
		return nil, apperr.ErrInternal
	}
	payload["published"] = published
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if err := guardPublication(it.Payload, body); err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdateItemPayloadIfUnchanged(ctx, itemID, it.Payload, body)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrMediaJobConflict
	}
	it.Payload = body
	return it, nil
}

// ensureMediaLesson finds (or lazily creates) the hidden media bucket and
// returns a lesson id under it to attach media items to.
func (s *Service) ensureMediaLesson(ctx context.Context) (string, error) {
	courses, err := s.repo.ListAllCourses(ctx)
	if err != nil {
		return "", err
	}
	var courseID string
	for i := range courses {
		if courses[i].Title == MediaBucketTitle {
			courseID = courses[i].ID
			break
		}
	}
	if courseID == "" {
		c := &coursedomain.Course{Language: "en", CEFR: "A1", Title: MediaBucketTitle, Description: "System bucket for the media library.", Published: false}
		if err := s.repo.CreateCourse(ctx, c); err != nil {
			return "", err
		}
		courseID = c.ID
	}
	units, err := s.repo.Units(ctx, courseID)
	if err != nil {
		return "", err
	}
	var unitID string
	if len(units) > 0 {
		unitID = units[0].ID
	} else {
		u := &coursedomain.Unit{CourseID: courseID, Title: "Media"}
		if err := s.repo.CreateUnit(ctx, u); err != nil {
			return "", err
		}
		unitID = u.ID
	}
	lessons, err := s.repo.LessonsByUnit(ctx, unitID)
	if err != nil {
		return "", err
	}
	if len(lessons) > 0 {
		return lessons[0].ID, nil
	}
	l := &coursedomain.Lesson{UnitID: unitID, Title: "Media"}
	if err := s.repo.CreateLesson(ctx, l); err != nil {
		return "", err
	}
	return l.ID, nil
}

var languageCode = regexp.MustCompile(`^[a-z]{2,3}$`)

// languageKnown reports whether code is in the languages table (enabled or
// not: content for a disabled language can be prepared before it goes live).
func (s *Service) languageKnown(ctx context.Context, code string) bool {
	langs, err := s.repo.Languages(ctx)
	if err != nil {
		return false
	}
	for _, l := range langs {
		if l.Code == code {
			return true
		}
	}
	return false
}

// Languages lists every target language (enabled and disabled) for admins.
func (s *Service) Languages(ctx context.Context) ([]coursedomain.Language, error) {
	return s.repo.Languages(ctx)
}

// CreateLanguage adds a target language. It starts disabled so its courses can
// be built and checked before learners see it, and is placed last.
func (s *Service) CreateLanguage(ctx context.Context, code string) (*coursedomain.Language, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if !languageCode.MatchString(code) {
		return nil, coursedomain.ErrLanguageCode
	}
	langs, err := s.repo.Languages(ctx)
	if err != nil {
		return nil, err
	}
	last := 0
	for _, l := range langs {
		if l.Code == code {
			return nil, coursedomain.ErrLanguageExists
		}
		if l.Position > last {
			last = l.Position
		}
	}
	l := coursedomain.Language{Code: code, Enabled: false, Position: last + 1}
	if err := s.repo.SaveLanguage(ctx, l); err != nil {
		return nil, err
	}
	return &l, nil
}

// UpdateLanguage enables/disables a language and/or moves it in the order.
func (s *Service) UpdateLanguage(ctx context.Context, code string, enabled *bool, position *int) (*coursedomain.Language, error) {
	langs, err := s.repo.Languages(ctx)
	if err != nil {
		return nil, err
	}
	for _, l := range langs {
		if l.Code != code {
			continue
		}
		if enabled != nil {
			l.Enabled = *enabled
		}
		if position != nil {
			l.Position = *position
		}
		if err := s.repo.SaveLanguage(ctx, l); err != nil {
			return nil, err
		}
		return &l, nil
	}
	return nil, coursedomain.ErrLanguageNotFound
}
