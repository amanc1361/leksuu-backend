// Package usecase holds assignment logic: a tutor creates work for a student,
// the student submits a text response, and the tutor grades it with a score and
// feedback. Names are resolved via a user directory for friendly display.
package usecase

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/leksuu/platform/internal/modules/assignment/domain"
	"github.com/leksuu/platform/internal/shared/apperr"
	"github.com/leksuu/platform/internal/shared/id"
)

// TutorCheck reports whether a user is a tutor (only tutors may assign work).
type TutorCheck interface {
	IsTutor(ctx context.Context, userID string) (bool, error)
}

// UserDirectory resolves user ids to display names.
type UserDirectory interface {
	DisplayNames(ctx context.Context, ids []string) (map[string]string, error)
}

// Notifier emits in-app notifications when events happen (optional).
type Notifier interface {
	Notify(ctx context.Context, userID, ntype string, data map[string]string, link string) error
}

type Service struct {
	repo   domain.Repository
	tutors TutorCheck
	users  UserDirectory
	notif  Notifier
	clock  func() time.Time
}

// SetNotifier wires the notification emitter (optional; nil-safe).
func (s *Service) SetNotifier(n Notifier) { s.notif = n }

func (s *Service) emit(ctx context.Context, userID, ntype string, data map[string]string, link string) {
	if s.notif != nil {
		_ = s.notif.Notify(ctx, userID, ntype, data, link)
	}
}

func NewService(repo domain.Repository, tutors TutorCheck, users UserDirectory, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, tutors: tutors, users: users, clock: clock}
}

func (s *Service) now() time.Time { return s.clock().UTC() }

func (s *Service) names(ctx context.Context, ids ...string) map[string]string {
	if s.users == nil {
		return map[string]string{}
	}
	m, _ := s.users.DisplayNames(ctx, ids)
	if m == nil {
		return map[string]string{}
	}
	return m
}

// View is an assignment plus the derived status and the relevant party's name.
type View struct {
	domain.Assignment
	Status      string `json:"status"`
	StudentName string `json:"student_name,omitempty"`
	TutorName   string `json:"tutor_name,omitempty"`
}

type CreateInput struct {
	StudentID string
	Type      string
	Title     string
	Prompt    string
	LessonID  string
	DueUTC    *time.Time
}

// Create lets a tutor assign work to a student.
func (s *Service) Create(ctx context.Context, tutorUserID string, in CreateInput) (*domain.Assignment, error) {
	if s.tutors != nil {
		ok, err := s.tutors.IsTutor(ctx, tutorUserID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apperr.New("tutor.not_a_tutor", apperr.KindForbidden)
		}
	}
	in.Title = strings.TrimSpace(in.Title)
	in.StudentID = strings.TrimSpace(in.StudentID)
	if in.StudentID == "" || in.Title == "" {
		return nil, apperr.ErrBadRequest
	}
	if in.Type != domain.TypeTask && in.Type != domain.TypeLesson {
		return nil, apperr.New("assignment.bad_type", apperr.KindValidation)
	}
	if in.Type == domain.TypeTask && strings.TrimSpace(in.Prompt) == "" {
		return nil, apperr.New("assignment.prompt_required", apperr.KindValidation)
	}
	if in.Type == domain.TypeLesson && strings.TrimSpace(in.LessonID) == "" {
		return nil, apperr.New("assignment.lesson_required", apperr.KindValidation)
	}
	a := &domain.Assignment{
		ID:        id.New(),
		TutorID:   tutorUserID,
		StudentID: in.StudentID,
		Type:      in.Type,
		Title:     in.Title,
		Prompt:    strings.TrimSpace(in.Prompt),
		LessonID:  strings.TrimSpace(in.LessonID),
		DueUTC:    in.DueUTC,
		CreatedAt: s.now(),
		Score:     -1,
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	tn := s.names(ctx, tutorUserID)[tutorUserID]
	s.emit(ctx, a.StudentID, domain.TypeNew, map[string]string{"tutor_name": tn, "title": a.Title}, "/homework")
	return a, nil
}

// ListForTutor returns the assignments a tutor created, with student names.
func (s *Service) ListForTutor(ctx context.Context, tutorUserID string) ([]View, error) {
	as, err := s.repo.ByTutor(ctx, tutorUserID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(as))
	for _, a := range as {
		ids = append(ids, a.StudentID)
	}
	nm := s.names(ctx, ids...)
	out := make([]View, 0, len(as))
	for _, a := range as {
		out = append(out, View{Assignment: a, Status: a.Status(), StudentName: nm[a.StudentID]})
	}
	return out, nil
}

// ListForStudent returns the assignments given to a student, with tutor names.
func (s *Service) ListForStudent(ctx context.Context, studentID string) ([]View, error) {
	as, err := s.repo.ByStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(as))
	for _, a := range as {
		ids = append(ids, a.TutorID)
	}
	nm := s.names(ctx, ids...)
	out := make([]View, 0, len(as))
	for _, a := range as {
		out = append(out, View{Assignment: a, Status: a.Status(), TutorName: nm[a.TutorID]})
	}
	return out, nil
}

// Submit records a student's text response (allowed until graded).
func (s *Service) Submit(ctx context.Context, studentID, assignmentID, text string) (*domain.Assignment, error) {
	a, err := s.repo.Get(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if a == nil || a.StudentID != studentID {
		return nil, apperr.New("assignment.not_found", apperr.KindNotFound)
	}
	if a.GradedAt != nil {
		return nil, apperr.New("assignment.already_graded", apperr.KindConflict)
	}
	if strings.TrimSpace(text) == "" {
		return nil, apperr.ErrBadRequest
	}
	now := s.now()
	a.SubmissionText = strings.TrimSpace(text)
	a.SubmittedAt = &now
	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Grade records the tutor's score (0-100) and feedback for a submitted task.
func (s *Service) Grade(ctx context.Context, tutorUserID, assignmentID string, score int, feedback string) (*domain.Assignment, error) {
	a, err := s.repo.Get(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if a == nil || a.TutorID != tutorUserID {
		return nil, apperr.New("assignment.not_found", apperr.KindNotFound)
	}
	if a.SubmittedAt == nil {
		return nil, apperr.New("assignment.not_submitted", apperr.KindValidation)
	}
	if score < 0 || score > 100 {
		return nil, apperr.New("assignment.bad_score", apperr.KindValidation)
	}
	now := s.now()
	a.Score = score
	a.Feedback = strings.TrimSpace(feedback)
	a.GradedAt = &now
	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	s.emit(ctx, a.StudentID, domain.TypeGraded, map[string]string{"title": a.Title, "score": strconv.Itoa(score)}, "/homework")
	return a, nil
}

// StudentStats summarizes a student's homework across all their assignments.
type StudentStats struct {
	Assigned  int `json:"assigned"`
	Submitted int `json:"submitted"`
	Graded    int `json:"graded"`
	AvgScore  int `json:"avg_score"`
}

// StatsForStudent aggregates assignment counts and the average graded score.
func (s *Service) StatsForStudent(ctx context.Context, studentID string) (StudentStats, error) {
	as, err := s.repo.ByStudent(ctx, studentID)
	if err != nil {
		return StudentStats{}, err
	}
	var st StudentStats
	sum, n := 0, 0
	for _, a := range as {
		switch a.Status() {
		case domain.StatusGraded:
			st.Graded++
			sum += a.Score
			n++
		case domain.StatusSubmitted:
			st.Submitted++
		default:
			st.Assigned++
		}
	}
	if n > 0 {
		st.AvgScore = sum / n
	}
	return st, nil
}

// Get returns one assignment if the caller is its tutor or its student.
func (s *Service) Get(ctx context.Context, userID, assignmentID string) (*View, error) {
	a, err := s.repo.Get(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if a == nil || (a.TutorID != userID && a.StudentID != userID) {
		return nil, apperr.New("assignment.not_found", apperr.KindNotFound)
	}
	nm := s.names(ctx, a.TutorID, a.StudentID)
	return &View{Assignment: *a, Status: a.Status(), StudentName: nm[a.StudentID], TutorName: nm[a.TutorID]}, nil
}
