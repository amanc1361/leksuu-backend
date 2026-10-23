// Package app is the composition root. It wires the shared kernel, builds each
// feature module from configuration, mounts their routes, and owns the process
// lifecycle. Adding a module means a few lines here — never a change to main.
package app

import (
	"context"
	"errors"
	classroomdomain "github.com/leksuu/platform/internal/modules/classroom/domain"
	tutordomain "github.com/leksuu/platform/internal/modules/tutor/domain"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	admin "github.com/leksuu/platform/internal/modules/admin"
	ai "github.com/leksuu/platform/internal/modules/ai"
	assignment "github.com/leksuu/platform/internal/modules/assignment"
	auth "github.com/leksuu/platform/internal/modules/auth"
	authusecase "github.com/leksuu/platform/internal/modules/auth/usecase"
	billing "github.com/leksuu/platform/internal/modules/billing"
	billingdomain "github.com/leksuu/platform/internal/modules/billing/domain"
	billingusecase "github.com/leksuu/platform/internal/modules/billing/usecase"
	classroom "github.com/leksuu/platform/internal/modules/classroom"
	classroomusecase "github.com/leksuu/platform/internal/modules/classroom/usecase"
	course "github.com/leksuu/platform/internal/modules/course"
	courseusecase "github.com/leksuu/platform/internal/modules/course/usecase"
	dictionary "github.com/leksuu/platform/internal/modules/dictionary"
	gamification "github.com/leksuu/platform/internal/modules/gamification"
	gamificationusecase "github.com/leksuu/platform/internal/modules/gamification/usecase"
	media "github.com/leksuu/platform/internal/modules/media"
	"github.com/leksuu/platform/internal/modules/minimalpairs"
	minimalpairsrepo "github.com/leksuu/platform/internal/modules/minimalpairs/repo"
	notification "github.com/leksuu/platform/internal/modules/notification"
	placement "github.com/leksuu/platform/internal/modules/placement"
	pronunciation "github.com/leksuu/platform/internal/modules/pronunciation"
	pronusecase "github.com/leksuu/platform/internal/modules/pronunciation/usecase"
	"github.com/leksuu/platform/internal/modules/recording"
	recordingusecase "github.com/leksuu/platform/internal/modules/recording/usecase"
	reminder "github.com/leksuu/platform/internal/modules/reminder"
	rtc "github.com/leksuu/platform/internal/modules/rtc"
	settings "github.com/leksuu/platform/internal/modules/settings"
	settingsusecase "github.com/leksuu/platform/internal/modules/settings/usecase"
	shadowing "github.com/leksuu/platform/internal/modules/shadowing"
	srs "github.com/leksuu/platform/internal/modules/srs"
	srsusecase "github.com/leksuu/platform/internal/modules/srs/usecase"
	studentreport "github.com/leksuu/platform/internal/modules/studentreport"
	tutor "github.com/leksuu/platform/internal/modules/tutor"
	tutorusecase "github.com/leksuu/platform/internal/modules/tutor/usecase"
	"github.com/leksuu/platform/internal/platform/config"
	"github.com/leksuu/platform/internal/platform/httpserver"
	"github.com/leksuu/platform/internal/shared/web"
)

const apiPrefix = "/api/v1"

type App struct {
	cfg          config.Config
	server       *httpserver.Server
	cleanup      func()
	log          *slog.Logger
	scheduler    *reminder.Scheduler
	recordings   *recordingusecase.Service
	gamification *gamificationusecase.Service
}

func New(cfg config.Config) (*App, error) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.IsProduction() && len(cfg.JWTSecret) < 32 {
		log.Warn("config: JWT_SECRET is shorter than 32 characters; use a long random secret")
	}

	repos, cleanup, err := buildRepos(cfg)
	if err != nil {
		return nil, err
	}

	mediaStore, err := newMediaStore(cfg)
	if err != nil {
		return nil, err
	}

	// --- modules ---
	authModule := auth.New(auth.Deps{
		Users:       repos.authUsers,
		Tokens:      repos.authTokens,
		JWTSecret:   []byte(cfg.JWTSecret),
		AccessTTL:   cfg.AccessTTL,
		RefreshTTL:  cfg.RefreshTTL,
		Clock:       time.Now,
		AdminEmails: cfg.AdminEmails,
	})
	// Make the configured admin account(s) usable out of the box (memory + pg).
	// Production never creates an admin with the published default password:
	// set ADMIN_PASSWORD to bootstrap one (existing admins keep their role).
	if cfg.IsProduction() && (cfg.AdminPassword == "" || cfg.AdminPassword == config.DefaultAdminPassword) {
		log.Warn("auth: ADMIN_PASSWORD is unset or the default; admin bootstrap skipped")
	} else {
		authModule.BootstrapAdmins(context.Background(), cfg.AdminPassword)
	}

	notifModule := notification.New(notification.Deps{
		Repo:   repos.notification,
		Guard:  authModule.Authenticator(),
		Tokens: authModule.Authenticator(),
		Clock:  time.Now,
	})

	placementModule := placement.New(placement.Deps{
		Repo:  repos.placement,
		Guard: authModule.LearnerGuard(),
	})

	gamificationModule := gamification.New(gamification.Deps{
		Repo:  repos.gamification,
		Guard: authModule.LearnerGuard(),
		Clock: time.Now,
	})

	dictionaryModule := dictionary.New(dictionary.Deps{
		Repo:  repos.dict,
		Guard: authModule.Authenticator(),
	})

	minimalpairsModule := minimalpairs.New(minimalpairs.Deps{Repo: minimalpairsrepo.NewMemoryRepo()})

	shadowingModule := shadowing.New(shadowing.Deps{
		Repo:     repos.shadowing,
		Rewarder: gamificationModule.Rewards(),
		Guard:    authModule.LearnerGuard(),
	})

	aiModule := ai.New(ai.Deps{
		ProviderKind:    cfg.AIProvider,
		BaseURL:         cfg.AIBaseURL,
		Model:           cfg.AIModel,
		APIKey:          cfg.AIAPIKey,
		DisableThinking: cfg.AIDisableThinking,
		Guard:           authModule.LearnerGuard(),
	})

	settingsModule := settings.New(settings.Deps{
		Repo:     repos.settings,
		PlanRepo: repos.settingsPlans,
		Guard:    authModule.Authenticator(),
	})

	billingModule := billing.New(billing.Deps{
		ProviderKind:    cfg.PaymentProvider,
		MerchantID:      cfg.ZarinpalMerchantID,
		ZarinpalSandbox: cfg.ZarinpalSandbox,
		// The stub approves every payment without charging; never in production.
		AllowStub: !cfg.IsProduction(),
		Repo:      repos.billing,
		Wallet:    gamificationModule.Wallet(),
		Config: billingusecase.Config{
			Plans: []billingdomain.PlanOption{
				{ID: "monthly", Title: "billing.monthly", AmountToman: cfg.PriceMonthlyToman, Days: 30},
				{ID: "yearly", Title: "billing.yearly", AmountToman: cfg.PriceYearlyToman, Days: 365},
			},
			FreeDailyLimit: cfg.FreeDailyLimit,
			GemUnlockCost:  cfg.GemUnlockCost,
			Action:         "lesson",
			CallbackURL:    cfg.BillingCallbackURL,
			SuccessURL:     cfg.BillingSuccessURL,
			FailURL:        cfg.BillingFailURL,
		},
		Guard: authModule.Authenticator(),
	})
	// The AI coach is not free: it needs the learning subscription.
	aiModule.SetPremium(billingModule.Service())
	billingModule.Service().SetSettings(billingSettings{settingsModule.Service()})
	switch billingModule.Service().ProviderName() {
	case "stub":
		log.Warn("billing: PAYMENT_PROVIDER is the stub; purchases are simulated (development only)")
	case "disabled":
		log.Warn("billing: no payment gateway configured (PAYMENT_PROVIDER / ZARINPAL_MERCHANT_ID); purchases are disabled")
	}
	authModule.Service().SetSubscriptionLookup(billingModule.Service())

	pronModule := pronunciation.New(pronunciation.Deps{
		ProviderKind: cfg.PronProvider,
		URL:          cfg.PronURL,
		APIKey:       cfg.PronAPIKey,
		Repo:         repos.pron,
		Rewarder:     gamificationModule.Rewards(),
		Premium:      billingModule.Service(),
		Guard:        authModule.LearnerGuard(),
	})

	adminModule := admin.New(admin.Deps{
		Repo:              repos.authoring,
		Vocab:             repos.srs,
		Guard:             authModule.Authenticator(),
		LocalizationURL:   cfg.LocalizationURL,
		LocalizationToken: cfg.LocalizationToken,
	})

	courseModule := course.New(course.Deps{
		Courses:  repos.courses,
		Progress: repos.progress,
		Rewarder: gamificationModule.Rewards(), // completing lessons/exercises grants XP
		Premium:  billingModule.Service(),
		Guard:    authModule.Authenticator(), // protected routes reuse auth middleware
		Clock:    time.Now,
	})
	// A learner's target language must be one the admin has enabled.
	authModule.Service().SetTargetLangCheck(courseModule.Service().LanguageEnabled)
	billingModule.Service().SetAccessResolver(lessonAccessResolver{courseModule.Service()})
	// Locked lesson content is withheld, graded and completed only with access.
	courseModule.Service().SetEntitlements(billingModule.Service())
	// Speak exercises are graded from the server's own pronunciation scores.
	courseModule.Service().SetSpeakingScores(speakingScores{pronModule.Service()})
	if cfg.IsProduction() && cfg.PronProvider == "stub" {
		log.Warn("pronunciation: PRON_PROVIDER is the stub; speaking scores do not analyse audio")
	}

	mediaModule := media.New(media.Deps{
		Store:    mediaStore,
		Guard:    authModule.Authenticator(),
		MaxBytes: cfg.MediaMaxBytes,
	})

	srsModule := srs.New(srs.Deps{
		Repo:     repos.srs,
		Guard:    authModule.LearnerGuard(),
		Rewarder: gamificationModule.Rewards(), // a review grants a little XP
		Clock:    time.Now,
	})
	// A missed grammar exercise enrolls its rule into spaced repetition.
	courseModule.Service().SetGrammarReviewer(grammarEnroller{srsModule.Service()})
	// The daily-mix bonus requires the learner's review queue to be empty.
	gamificationModule.Service().SetReviewQueue(reviewQueue{srsModule.Service()})

	tutorModule := tutor.New(tutor.Deps{
		Repo:  repos.tutor,
		Guard: authModule.Authenticator(),
		Clock: time.Now,
		Users: authModule.Service(), // resolve student display names for the portal
	})
	// Online classes are sold per session: prices, credit and checkouts live in
	// the tutor store; billing opens the gateway payment and settles it.
	if m, ok := repos.tutor.(tutordomain.MarketStore); ok {
		tutorModule.Service().SetMarket(m)
	}
	tutorModule.Service().SetShare(settingsModule.Service())
	tutorModule.Service().SetPayments(billingModule.Service())
	billingModule.Service().SetSessionFulfiller(tutorModule.Service())
	tutorModule.Service().SetNotifier(notifModule.Service())
	gamificationModule.Service().SetNotifier(notifModule.Service()) // weekly recap
	tutorModule.Service().SetClassAccess(billingModule.Service())
	tutorModule.Service().SetJoinWindow(settingsModule.Service())
	tutorModule.Service().SetPayoutRate(settingsModule.Service())
	tutorModule.Service().SetRoles(authModule.Service())

	classroomModule := classroom.New(classroom.Deps{
		Repo:   repos.classroom,
		Guard:  authModule.Authenticator(),
		Tutors: tutorModule.Service(),
		Users:  authModule.Service(),
		Clock:  time.Now,
	})
	// Group classes are sold per seat the same way (credit shared with 1:1).
	if m, ok := repos.classroom.(classroomdomain.MarketStore); ok {
		classroomModule.Service().SetMarket(m)
	}
	classroomModule.Service().SetCredits(tutorModule.Service())
	classroomModule.Service().SetPayments(billingModule.Service())
	classroomModule.Service().SetShare(settingsModule.Service())
	billingModule.Service().SetFulfiller("group_class", classroomModule.Service())
	classroomModule.Service().SetNotifier(notifModule.Service())
	classroomModule.Service().SetClassAccess(billingModule.Service())
	classroomModule.Service().SetJoinWindow(settingsModule.Service())
	classroomModule.Service().SetSessionSource(tutorModule.Service())
	gamificationModule.Service().SetNames(authModule.Service().DisplayNames) // weekly league shows real names
	placementModule.Service().SetLevelSink(func(ctx context.Context, userID, cefr string) error {
		_, err := authModule.Service().UpdateProfile(ctx, userID, authusecase.UpdateProfileInput{CEFR: cefr})
		return err
	}) // tested level becomes the profile level everywhere

	mediaModule.SetAvatarGate(func(ctx context.Context, userID string) bool {
		t, err := tutorModule.Service().TutorForUser(ctx, userID)
		return err == nil && t != nil
	}) // approved teachers may upload a listing avatar
	tutorModule.Service().SetGroupSessions(groupSessionsAdapter{classroomModule.Service()})

	recordingModule := recording.New(recording.Deps{
		Repo:   repos.recording,
		Auth:   classroomModule.Service(),
		Store:  mediaStore,
		TmpDir: cfg.RecordingsDir,
		Guard:  authModule.Authenticator(),
	})
	recordingModule.Service().SetRetentionDays(cfg.RecordingRetentionDays)

	// TURN relays media when a direct path can't be negotiated (symmetric NAT,
	// restrictive mobile networks). Without it those calls connect on the
	// signalling channel but never exchange video — a silent, hard-to-trace
	// failure — so make the gap explicit at boot.
	if cfg.TurnURL == "" && cfg.TurnTLSURL == "" {
		log.Warn("rtc: no TURN server configured; calls behind restrictive NATs will connect without media (set TURN_URL/TURN_TLS_URL and TURN_SECRET)")
	}
	if cfg.StunURL == "" {
		log.Warn("rtc: no STUN server configured; remote video will not work across different networks (set STUN_URL)")
	}

	rtcModule := rtc.New(rtc.Deps{
		Guard:       authModule.Authenticator(),
		TokenParser: authModule.Authenticator(),
		Authorizer:  compositeAuthorizer{[]roomAuthorizer{tutorModule.Service(), classroomModule.Service()}},
		Identifier:  rtcIdentifier{names: authModule.Service().DisplayNames},
		StunURL:     cfg.StunURL,
		TurnURL:     cfg.TurnURL,
		TurnTLSURL:  cfg.TurnTLSURL,
		TurnSecret:  cfg.TurnSecret,
		TurnTTL:     cfg.TurnTTL,
	})

	assignmentModule := assignment.New(assignment.Deps{
		Repo:   repos.assignment,
		Guard:  authModule.Authenticator(),
		Tutors: tutorModule.Service(),
		Users:  authModule.Service(),
		Clock:  time.Now,
	})
	assignmentModule.Service().SetNotifier(notifModule.Service())

	reportModule := studentreport.New(studentreport.Deps{
		Guard:       authModule.Authenticator(),
		Auth:        tutorModule.Service(),
		Names:       authModule.Service(),
		Game:        gamificationModule.Service(),
		SRS:         srsModule.Service(),
		Pron:        pronModule.Service(),
		Courses:     courseModule.Service(),
		Assignments: assignmentModule.Service(),
	})

	// --- routing ---
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","env":"` + cfg.Env + `"}`))
	})
	authModule.Mount(mux, apiPrefix)
	courseModule.Mount(mux, apiPrefix)
	gamificationModule.Mount(mux, apiPrefix)
	placementModule.Mount(mux, apiPrefix)
	adminModule.Mount(mux, apiPrefix)
	dictionaryModule.Mount(mux, apiPrefix)
	minimalpairsModule.Mount(mux, apiPrefix)
	shadowingModule.Mount(mux, apiPrefix)
	aiModule.Mount(mux, apiPrefix)
	pronModule.Mount(mux, apiPrefix)
	billingModule.Mount(mux, apiPrefix)
	settingsModule.Mount(mux, apiPrefix)
	mediaModule.Mount(mux, apiPrefix)
	srsModule.Mount(mux, apiPrefix)
	tutorModule.Mount(mux, apiPrefix)
	classroomModule.Mount(mux, apiPrefix)
	recordingModule.Mount(mux, apiPrefix)
	rtcModule.Mount(mux, apiPrefix)
	assignmentModule.Mount(mux, apiPrefix)
	notifModule.Mount(mux, apiPrefix)
	reportModule.Mount(mux, apiPrefix)

	// Class-reminder scheduler: notify participants ~15 min before a session starts.
	remindScheduler := reminder.NewScheduler(
		notifModule.Service(),
		[]reminder.Source{tutorModule.Service(), classroomModule.Service()},
		time.Duration(cfg.ReminderLeadMin)*time.Minute,
		time.Duration(cfg.ReminderIntervalSec)*time.Second,
		time.Now,
	)

	return &App{
		cfg:          cfg,
		server:       httpserver.New(":"+cfg.HTTPPort, web.CORS(cfg.CORSOrigins, mux)),
		cleanup:      cleanup,
		log:          log,
		scheduler:    remindScheduler,
		recordings:   recordingModule.Service(),
		gamification: gamificationModule.Service(),
	}, nil
}

func (a *App) Run() error {
	defer a.cleanup()

	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	a.scheduler.Start(schedCtx)
	a.recordings.StartCleanup(schedCtx, 12*time.Hour)     // auto-purge expired recordings
	a.gamification.StartWeeklyRecaps(schedCtx, time.Hour) // once-per-week recap, hourly check

	errCh := make(chan error, 1)
	go func() {
		a.log.Info("server starting", "port", a.cfg.HTTPPort, "env", a.cfg.Env)
		if err := a.server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
		a.log.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(ctx)
	}
}

// roomAuthorizer authorizes a user joining a live room (1:1 booking or group
// class). The rtc module accepts one Authorizer, so we compose several: the
// first that authorizes (or errors) wins.
type roomAuthorizer interface {
	CanJoin(ctx context.Context, userID, id string) (bool, string, error)
}

// rtcIdentifier resolves a user's display name for the live-room roster/chat,
// adapting the auth service's batch DisplayNames lookup to the rtc Identifier.
type rtcIdentifier struct {
	names func(ctx context.Context, ids []string) (map[string]string, error)
}

func (i rtcIdentifier) DisplayName(ctx context.Context, userID string) string {
	if i.names == nil {
		return ""
	}
	m, err := i.names(ctx, []string{userID})
	if err != nil {
		return ""
	}
	return m[userID]
}

type compositeAuthorizer struct{ authorizers []roomAuthorizer }

func (c compositeAuthorizer) CanJoin(ctx context.Context, userID, id string) (bool, string, error) {
	for _, a := range c.authorizers {
		ok, room, err := a.CanJoin(ctx, userID, id)
		if err != nil {
			return false, "", err
		}
		if ok {
			return true, room, nil
		}
	}
	return false, "", nil
}

// billingSettings adapts the settings service to billing's SettingsProvider,
// mapping the live pricing into billing's own plan type.
type billingSettings struct{ s *settingsusecase.Service }

func (b billingSettings) Pricing(ctx context.Context) (billingusecase.Pricing, error) {
	pr, err := b.s.Pricing(ctx)
	if err != nil {
		return billingusecase.Pricing{}, err
	}
	conv := func(in []settingsusecase.PlanOptionView) []billingdomain.PlanOption {
		out := make([]billingdomain.PlanOption, 0, len(in))
		for _, p := range in {
			out = append(out, billingdomain.PlanOption{ID: p.ID, Title: p.Title, AmountToman: p.AmountToman, Days: p.Days})
		}
		return out
	}
	return billingusecase.Pricing{
		FreeDailyLimit: pr.FreeDailyLimit, GemUnlockCost: pr.GemUnlockCost,
		CoursePlans: conv(pr.CoursePlans), ClassPlans: conv(pr.ClassPlans),
	}, nil
}

// lessonAccessResolver adapts the course service to billing's AccessResolver.
// grammarEnroller adapts the srs service to the course module's GrammarReviewer
// port: a missed grammar exercise becomes a spaced-repetition card for its rule.
type grammarEnroller struct{ s *srsusecase.Service }

func (g grammarEnroller) EnrollGrammar(ctx context.Context, userID, language, pattern string) error {
	_, err := g.s.AddCard(ctx, userID, srsusecase.AddInput{
		Language: language,
		Kind:     "grammar",
		Lemma:    pattern,
		Source:   "grammar",
		Meaning: map[string]string{
			"fa":  "قاعده‌ی گرامری برای مرور",
			"ckb": "ڕێسای ڕێزمان بۆ پێداچوونەوە",
			"en":  "Grammar rule to review",
		},
	})
	return err
}

type lessonAccessResolver struct{ s *courseusecase.Service }

func (r lessonAccessResolver) LessonRequirement(ctx context.Context, lessonID string) (string, error) {
	return r.s.LessonRequirement(ctx, lessonID)
}

// speakingScores adapts the pronunciation service to the course module's
// SpeakingScores port: the score the server recorded for the learner's latest
// attempt at a speak exercise (stored under the exercise id, line 0).
type speakingScores struct{ s *pronusecase.Service }

func (p speakingScores) LatestScore(ctx context.Context, userID, exerciseID string) (int, bool, error) {
	scores, err := p.s.Scores(ctx, userID, exerciseID)
	if err != nil {
		return 0, false, err
	}
	for _, ls := range scores {
		if ls.Line == 0 && ls.Attempts > 0 {
			return ls.Last, true, nil
		}
	}
	return 0, false, nil
}

// reviewQueue adapts the srs service to gamification's ReviewQueue port.
type reviewQueue struct{ s *srsusecase.Service }

func (q reviewQueue) DueCount(ctx context.Context, userID string) (int, error) {
	st, err := q.s.Stats(ctx, userID)
	if err != nil {
		return 0, err
	}
	return st.Due, nil
}

// groupSessionsAdapter exposes a tutor's group classes to the earnings report.
type groupSessionsAdapter struct{ s *classroomusecase.Service }

func (g groupSessionsAdapter) TutorGroupSessions(ctx context.Context, tutorUserID string) ([]tutorusecase.GroupSession, error) {
	views, err := g.s.TutorClasses(ctx, tutorUserID)
	if err != nil {
		return nil, err
	}
	return groupViewsToSessions(views), nil
}

func (g groupSessionsAdapter) AllGroupSessions(ctx context.Context) ([]tutorusecase.GroupSession, error) {
	views, err := g.s.AllClasses(ctx)
	if err != nil {
		return nil, err
	}
	return groupViewsToSessions(views), nil
}

func (g groupSessionsAdapter) StudentGroupSessions(ctx context.Context, studentUserID string) ([]tutorusecase.GroupSession, error) {
	views, err := g.s.MyClasses(ctx, studentUserID)
	if err != nil {
		return nil, err
	}
	return groupViewsToSessions(views), nil
}

func groupViewsToSessions(views []classroomusecase.ClassView) []tutorusecase.GroupSession {
	now := time.Now().UTC()
	out := make([]tutorusecase.GroupSession, 0, len(views))
	for _, v := range views {
		out = append(out, tutorusecase.GroupSession{
			StartUTC: v.StartUTC, DurationMin: v.DurationMin, Title: v.Title,
			Attendees: v.Enrolled, Completed: v.EndUTC().Before(now),
			Priced: v.PriceToman > 0, RevenueToman: v.RevenueToman, TeacherEarnedToman: v.TeacherEarnedToman,
		})
	}
	return out
}
