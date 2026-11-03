package app

import (
	adminusecase "github.com/leksuu/platform/internal/modules/admin/usecase"
	assignmentdomain "github.com/leksuu/platform/internal/modules/assignment/domain"
	authdomain "github.com/leksuu/platform/internal/modules/auth/domain"
	billingdomain "github.com/leksuu/platform/internal/modules/billing/domain"
	classroomdomain "github.com/leksuu/platform/internal/modules/classroom/domain"
	coursedomain "github.com/leksuu/platform/internal/modules/course/domain"
	dictdomain "github.com/leksuu/platform/internal/modules/dictionary/domain"
	gamedomain "github.com/leksuu/platform/internal/modules/gamification/domain"
	notificationdomain "github.com/leksuu/platform/internal/modules/notification/domain"
	placementdomain "github.com/leksuu/platform/internal/modules/placement/domain"
	prondomain "github.com/leksuu/platform/internal/modules/pronunciation/domain"
	recordingdomain "github.com/leksuu/platform/internal/modules/recording/domain"
	settingsdomain "github.com/leksuu/platform/internal/modules/settings/domain"
	shadowdomain "github.com/leksuu/platform/internal/modules/shadowing/domain"
	srsdomain "github.com/leksuu/platform/internal/modules/srs/domain"
	tutordomain "github.com/leksuu/platform/internal/modules/tutor/domain"
)

// repositories groups every module's repositories. Declared here (untagged) so
// both the default and pgx builds share the type; the constructor that fills it
// is selected by build tag (repos_default.go vs repos_pgx.go).
type repositories struct {
	authUsers     authdomain.UserRepository
	authTokens    authdomain.TokenRepository
	courses       coursedomain.CourseRepository
	progress      coursedomain.ProgressRepository
	gamification  gamedomain.Repository
	authoring     adminusecase.Repository
	mediaJobs     adminusecase.MediaJobStore
	srs           srsdomain.Repository
	dict          dictdomain.Repository
	shadowing     shadowdomain.Repository
	pron          prondomain.Repository
	billing       billingdomain.Repository
	tutor         tutordomain.Repository
	assignment    assignmentdomain.Repository
	classroom     classroomdomain.Repository
	notification  notificationdomain.Repository
	placement     placementdomain.Repository
	settings      settingsdomain.Repository
	settingsPlans settingsdomain.PlanRepository
	recording     recordingdomain.Repository
}
