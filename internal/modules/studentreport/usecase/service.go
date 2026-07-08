// Package usecase aggregates a single student's progress across modules into one
// report for an authorized tutor. Ports match the existing module service
// signatures, so the real services satisfy them directly (no adapters). Every
// sub-fetch is best-effort: a failing source contributes zeros, never an error.
package usecase

import (
	"context"

	assignusecase "github.com/leksuu/platform/internal/modules/assignment/usecase"
	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	courseusecase "github.com/leksuu/platform/internal/modules/course/usecase"
	gameusecase "github.com/leksuu/platform/internal/modules/gamification/usecase"
	prondomain "github.com/leksuu/platform/internal/modules/pronunciation/domain"
	srsusecase "github.com/leksuu/platform/internal/modules/srs/usecase"
	"github.com/leksuu/platform/internal/shared/apperr"
)

type Gamification interface {
	GetState(ctx context.Context, userID string) (*gameusecase.StateView, error)
}
type SRS interface {
	Stats(ctx context.Context, userID string) (*srsusecase.Stats, error)
}
type Pron interface {
	Summary(ctx context.Context, userID string) (prondomain.Summary, error)
}
type Courses interface {
	ListCourses(ctx context.Context, language, cefr string) ([]coursedomain.Course, error)
	CourseProgress(ctx context.Context, userID, courseID string) (*courseusecase.ProgressSummary, error)
}
type Assignments interface {
	StatsForStudent(ctx context.Context, studentID string) (assignusecase.StudentStats, error)
}
type TutorAuth interface {
	IsStudentOf(ctx context.Context, tutorUserID, studentID string) (bool, error)
}
type Names interface {
	DisplayNames(ctx context.Context, ids []string) (map[string]string, error)
}

type Service struct {
	auth    TutorAuth
	names   Names
	game    Gamification
	srs     SRS
	pron    Pron
	courses Courses
	assigns Assignments
}

type Deps struct {
	Auth        TutorAuth
	Names       Names
	Game        Gamification
	SRS         SRS
	Pron        Pron
	Courses     Courses
	Assignments Assignments
}

func NewService(d Deps) *Service {
	return &Service{auth: d.Auth, names: d.Names, game: d.Game, srs: d.SRS, pron: d.Pron, courses: d.Courses, assigns: d.Assignments}
}

// StudentReport is the unified progress snapshot shown to a tutor.
type StudentReport struct {
	StudentID string `json:"student_id"`
	Name      string `json:"name"`

	XP     int    `json:"xp"`
	Streak int    `json:"streak"`
	Gems   int    `json:"gems"`
	League string `json:"league"`

	SRSDue   int `json:"srs_due"`
	SRSTotal int `json:"srs_total"`

	PronScored   int `json:"pron_scored"`
	PronMastered int `json:"pron_mastered"`
	PronAvgBest  int `json:"pron_avg_best"`

	LessonsCompleted int `json:"lessons_completed"`
	LessonsTotal     int `json:"lessons_total"`
	Percent          int `json:"percent"`

	AssignAssigned  int `json:"assign_assigned"`
	AssignSubmitted int `json:"assign_submitted"`
	AssignGraded    int `json:"assign_graded"`
	AssignAvgScore  int `json:"assign_avg_score"`
}

// Build assembles the report. Returns forbidden if the student is not the
// tutor's student.
func (s *Service) Build(ctx context.Context, tutorUserID, studentID string) (*StudentReport, error) {
	ok, err := s.auth.IsStudentOf(ctx, tutorUserID, studentID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.New("tutor.not_your_student", apperr.KindForbidden)
	}

	r := &StudentReport{StudentID: studentID}
	if s.names != nil {
		if nm, err := s.names.DisplayNames(ctx, []string{studentID}); err == nil {
			r.Name = nm[studentID]
		}
	}
	if s.game != nil {
		if st, err := s.game.GetState(ctx, studentID); err == nil && st != nil {
			r.XP, r.Streak, r.Gems, r.League = st.State.XP, st.State.Streak, st.State.Gems, st.State.League
		}
	}
	if s.srs != nil {
		if ss, err := s.srs.Stats(ctx, studentID); err == nil && ss != nil {
			r.SRSDue, r.SRSTotal = ss.Due, ss.Total
		}
	}
	if s.pron != nil {
		if ps, err := s.pron.Summary(ctx, studentID); err == nil {
			r.PronScored, r.PronMastered, r.PronAvgBest = ps.Scored, ps.Mastered, ps.AvgBest
		}
	}
	if s.courses != nil {
		if cs, err := s.courses.ListCourses(ctx, "", ""); err == nil && len(cs) > 0 {
			if cp, err := s.courses.CourseProgress(ctx, studentID, cs[0].ID); err == nil && cp != nil {
				r.LessonsCompleted, r.LessonsTotal, r.Percent = cp.CompletedLessons, cp.TotalLessons, cp.Percent
			}
		}
	}
	if s.assigns != nil {
		if as, err := s.assigns.StatsForStudent(ctx, studentID); err == nil {
			r.AssignAssigned, r.AssignSubmitted, r.AssignGraded, r.AssignAvgScore = as.Assigned, as.Submitted, as.Graded, as.AvgScore
		}
	}
	return r, nil
}
