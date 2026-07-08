//go:build !pgx

package app

import (
	assignmentrepo "github.com/leksuu/platform/internal/modules/assignment/repo"
	authrepo "github.com/leksuu/platform/internal/modules/auth/repo"
	billingrepo "github.com/leksuu/platform/internal/modules/billing/repo"
	classroomrepo "github.com/leksuu/platform/internal/modules/classroom/repo"
	courserepo "github.com/leksuu/platform/internal/modules/course/repo"
	dictrepo "github.com/leksuu/platform/internal/modules/dictionary/repo"
	gamerepo "github.com/leksuu/platform/internal/modules/gamification/repo"
	notificationrepo "github.com/leksuu/platform/internal/modules/notification/repo"
	placementrepo "github.com/leksuu/platform/internal/modules/placement/repo"
	pronrepo "github.com/leksuu/platform/internal/modules/pronunciation/repo"
	recordingrepo "github.com/leksuu/platform/internal/modules/recording/repo"
	settingsrepo "github.com/leksuu/platform/internal/modules/settings/repo"
	shadowrepo "github.com/leksuu/platform/internal/modules/shadowing/repo"
	srsrepo "github.com/leksuu/platform/internal/modules/srs/repo"
	tutorrepo "github.com/leksuu/platform/internal/modules/tutor/repo"
	"github.com/leksuu/platform/internal/platform/config"
)

// buildRepos returns in-memory repositories (default build: dev & tests).
func buildRepos(_ config.Config) (repositories, func(), error) {
	courses := courserepo.NewMemoryCourseRepo()
	return repositories{
		authUsers:     authrepo.NewMemoryUserRepo(),
		authTokens:    authrepo.NewMemoryTokenRepo(),
		courses:       courses,
		progress:      courserepo.NewMemoryProgressRepo(courses.LessonCourseID),
		gamification:  gamerepo.NewMemoryRepo(),
		placement:     placementrepo.NewMemoryRepo(),
		authoring:     courses, // same store the course module reads
		srs:           srsrepo.NewMemorySRSRepo(),
		dict:          dictrepo.NewMemoryDictRepo(),
		shadowing:     shadowrepo.NewMemoryShadowRepo(),
		pron:          pronrepo.NewMemoryPronRepo(),
		billing:       billingrepo.NewMemoryBillingRepo(),
		tutor:         tutorrepo.NewMemoryTutorRepo(),
		assignment:    assignmentrepo.NewMemoryAssignmentRepo(),
		classroom:     classroomrepo.NewMemoryClassRepo(),
		recording:     recordingrepo.NewMemoryRepo(),
		notification:  notificationrepo.NewMemoryNotificationRepo(),
		settings:      settingsrepo.NewMemorySettingsRepo(),
		settingsPlans: settingsrepo.NewMemoryPlanRepo(),
	}, func() {}, nil
}
