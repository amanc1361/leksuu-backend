package usecase

import (
	"context"
	"testing"

	assignusecase "github.com/leksuu/platform/internal/modules/assignment/usecase"
	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	courseusecase "github.com/leksuu/platform/internal/modules/course/usecase"
	gamedomain "github.com/leksuu/platform/internal/modules/gamification/domain"
	gameusecase "github.com/leksuu/platform/internal/modules/gamification/usecase"
	prondomain "github.com/leksuu/platform/internal/modules/pronunciation/domain"
	srsusecase "github.com/leksuu/platform/internal/modules/srs/usecase"
)

type fAuth struct{ ok bool }

func (f fAuth) IsStudentOf(context.Context, string, string) (bool, error) { return f.ok, nil }

type fNames map[string]string

func (f fNames) DisplayNames(_ context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := f[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

type fGame struct{}

func (fGame) GetState(context.Context, string) (*gameusecase.StateView, error) {
	return &gameusecase.StateView{State: gamedomain.State{XP: 120, Streak: 4, Gems: 30, League: "silver"}}, nil
}

type fSRS struct{}

func (fSRS) Stats(context.Context, string) (*srsusecase.Stats, error) {
	return &srsusecase.Stats{Due: 5, Total: 40}, nil
}

type fPron struct{}

func (fPron) Summary(context.Context, string) (prondomain.Summary, error) {
	return prondomain.Summary{Scored: 12, Mastered: 7, AvgBest: 81}, nil
}

type fCourses struct{}

func (fCourses) ListCourses(context.Context, string, string) ([]coursedomain.Course, error) {
	return []coursedomain.Course{{ID: "c1"}}, nil
}
func (fCourses) CourseProgress(context.Context, string, string) (*courseusecase.ProgressSummary, error) {
	return &courseusecase.ProgressSummary{TotalLessons: 13, CompletedLessons: 6, Percent: 46}, nil
}

type fAssign struct{}

func (fAssign) StatsForStudent(context.Context, string) (assignusecase.StudentStats, error) {
	return assignusecase.StudentStats{Assigned: 1, Submitted: 2, Graded: 3, AvgScore: 88}, nil
}

func TestStudentReportBuild(t *testing.T) {
	ctx := context.Background()
	svc := NewService(Deps{
		Auth: fAuth{ok: true}, Names: fNames{"stu": "Ali"},
		Game: fGame{}, SRS: fSRS{}, Pron: fPron{}, Courses: fCourses{}, Assignments: fAssign{},
	})

	r, err := svc.Build(ctx, "tutor", "stu")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Ali" || r.XP != 120 || r.Streak != 4 || r.League != "silver" {
		t.Fatalf("game/name: %+v", r)
	}
	if r.SRSDue != 5 || r.SRSTotal != 40 {
		t.Fatalf("srs: %+v", r)
	}
	if r.PronScored != 12 || r.PronMastered != 7 || r.PronAvgBest != 81 {
		t.Fatalf("pron: %+v", r)
	}
	if r.LessonsCompleted != 6 || r.LessonsTotal != 13 || r.Percent != 46 {
		t.Fatalf("course: %+v", r)
	}
	if r.AssignGraded != 3 || r.AssignAvgScore != 88 {
		t.Fatalf("assign: %+v", r)
	}

	// Not the tutor's student -> forbidden.
	if _, err := NewService(Deps{Auth: fAuth{ok: false}}).Build(ctx, "tutor", "stu"); err == nil {
		t.Fatal("expected forbidden for non-student")
	}
}
