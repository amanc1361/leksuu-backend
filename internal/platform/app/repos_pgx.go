//go:build pgx

package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

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

// buildRepos connects to PostgreSQL and returns pgx-backed repositories.
func buildRepos(cfg config.Config) (repositories, func(), error) {
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return repositories{}, nil, err
	}
	courses := courserepo.NewPostgresCourseRepo(pool)
	return repositories{
		authUsers:     authrepo.NewPostgresUserRepo(pool),
		authTokens:    authrepo.NewPostgresTokenRepo(pool),
		courses:       courses,
		progress:      courserepo.NewPostgresProgressRepo(pool),
		gamification:  gamerepo.NewPostgresRepo(pool),
		placement:     placementrepo.NewPostgresRepo(pool),
		authoring:     courses, // admin authoring uses the same Postgres content store
		mediaJobs:     courserepo.NewPostgresMediaJobRepo(pool),
		srs:           srsrepo.NewPostgresSRSRepo(pool),
		dict:          dictrepo.NewPostgresDictRepo(pool),
		shadowing:     shadowrepo.NewPostgresShadowRepo(pool),
		billing:       billingrepo.NewPostgresBillingRepo(pool),
		pron:          pronrepo.NewPostgresPronRepo(pool),
		tutor:         tutorrepo.NewPostgresTutorRepo(pool),
		assignment:    assignmentrepo.NewPostgresAssignmentRepo(pool),
		classroom:     classroomrepo.NewPostgresClassRepo(pool),
		recording:     recordingrepo.NewPostgresRepo(pool),
		notification:  notificationrepo.NewPostgresNotificationRepo(pool),
		settings:      settingsrepo.NewPostgresSettingsRepo(pool),
		settingsPlans: settingsrepo.NewPostgresPlanRepo(pool),
	}, pool.Close, nil
}
