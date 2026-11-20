package router

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/activityapi"
	"github.com/kaptree/kapoj/apps/server/internal/adminapi"
	"github.com/kaptree/kapoj/apps/server/internal/agent"
	"github.com/kaptree/kapoj/apps/server/internal/agentapi"
	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/aiapi"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/backupapi"
	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/certificateapi"
	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/companionapi"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/contestapi"
	"github.com/kaptree/kapoj/apps/server/internal/difficulty"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/gamificationapi"
	"github.com/kaptree/kapoj/apps/server/internal/health"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/jobs"
	"github.com/kaptree/kapoj/apps/server/internal/jobsapi"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/learningapi"
	"github.com/kaptree/kapoj/apps/server/internal/mail"
	"github.com/kaptree/kapoj/apps/server/internal/mailapi"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/problemapi"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/ratingapi"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/similarity"
	"github.com/kaptree/kapoj/apps/server/internal/similarityapi"
	"github.com/kaptree/kapoj/apps/server/internal/site"
	"github.com/kaptree/kapoj/apps/server/internal/siteapi"
	"github.com/kaptree/kapoj/apps/server/internal/social"
	"github.com/kaptree/kapoj/apps/server/internal/socialapi"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/submissionapi"
	"github.com/kaptree/kapoj/apps/server/internal/submissionevents"
	"github.com/kaptree/kapoj/apps/server/internal/testcasegen"
	"github.com/kaptree/kapoj/apps/server/internal/user"
	"github.com/kaptree/kapoj/apps/server/internal/userapi"
	"github.com/kaptree/kapoj/apps/server/internal/userimport"
)

const (
	maxBodyBytes        = 1 << 20   // JSON endpoints
	maxProblemJSONBytes = 6 << 20   // problem create/update and inline testcase content
	maxUploadBytes      = 40 << 20  // file upload endpoints: two 16 MiB files plus form fields
	maxImportBodyBytes  = 210 << 20 // HydroOJ package upload: 200 MiB file plus multipart overhead
)

type Deps struct {
	DB          *gorm.DB
	Redis       *redis.Client
	Storage     storage.Storage
	Config      *config.Config
	Enqueuer    submission.Enqueuer
	Game        *gamification.Service
	Rating      *rating.Service
	Activity    *activity.Service
	Learning    *learning.Service
	RBAC        *rbac.Service
	AI          *ai.Service
	AISettings  *ai.SettingsService
	AIPlatform  *ai.PlatformService
	TestcaseGen *testcasegen.Service
	Certificate *certificate.Service
	Mail        *mail.Service
	Backup      *backup.Service
	Similarity  *similarity.Service
	Companion   *companion.Service
	// Difficulty wires the difficulty settings service into the problem API.
	// DifficultyAI/AISettings are seams for the platform assessor and the AI
	// feature toggle; production wiring lands in Task 11.
	Difficulty           *difficulty.SettingsService
	DifficultyAI         problemapi.DifficultyAssessor
	DifficultyAISettings problemapi.AISettings
	// Jobs wires the scheduled job service into the admin jobs API.
	Jobs *jobs.Service
	// Agent wires Agent Arena user routes into the authenticated API group.
	Agent *agent.Service
}

func New(d Deps) *gin.Engine {
	r := gin.New()
	if err := r.SetTrustedProxies(d.Config.TrustedProxies); err != nil {
		// Config validation rejects invalid values; a failure here must still
		// fail closed rather than trusting every forwarded header.
		slog.Warn("trusted proxies rejected; disabling proxy trust", "err", err)
		_ = r.SetTrustedProxies(nil)
	}
	r.Use(requestLog(), gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic recovered", "path", c.Request.URL.Path, "panic", recovered)
		httpx.Fail(c, httpx.ErrInternal)
	}))
	r.NoRoute(func(c *gin.Context) { httpx.Fail(c, httpx.ErrNotFound) })

	r.GET("/health", health.Handler(dbPinger{d.DB}, redisPinger{d.Redis}))

	users := user.NewGormRepository(d.DB)
	issuer := auth.NewTokenIssuer(d.Config.JWTSecret, d.Config.AccessTokenTTL)
	var authOpts []auth.ServiceOption
	if d.Mail != nil {
		authOpts = append(authOpts,
			auth.WithMailer(d.Mail),
			auth.WithEmailTokens(auth.NewEmailTokenStore(d.Redis)))
	}
	authSvc := auth.NewService(users, issuer,
		auth.NewRefreshStore(d.Redis, d.Config.RefreshTokenTTL),
		auth.NewLoginLimiter(d.Redis), authOpts...)

	var loginHooks []auth.LoginHook
	if d.Game != nil {
		loginHooks = append(loginHooks, d.Game)
	}

	// Auth endpoints identify the caller by the request body, not the bearer
	// token, so an expired access token must not block logout or refresh.
	auth.RegisterRoutes(r.Group("/api", middleware.MaxBody(maxBodyBytes)), authSvc, loginHooks...)

	public := r.Group("/api", middleware.MaxBody(maxBodyBytes))
	authenticated := func(limit int64) []gin.HandlerFunc {
		handlers := []gin.HandlerFunc{middleware.Authenticate(issuer)}
		if d.Mail != nil {
			handlers = append(handlers, middleware.RequireVerifiedEmail(d.Mail, users))
		}
		return append(handlers, middleware.MaxBody(limit))
	}
	api := r.Group("/api", authenticated(maxBodyBytes)...)
	auth.RegisterAccountRoutes(api, authSvc)
	if d.Agent != nil {
		agentapi.RegisterUserRoutes(api, d.Agent)
		agentapi.RegisterPublicRoutes(api, d.Agent)
		agentapi.RegisterAgentRoutes(public, d.Agent)
	}
	problemJSON := r.Group("/api", authenticated(maxProblemJSONBytes)...)
	uploads := r.Group("/api", authenticated(maxUploadBytes)...)
	imports := r.Group("/api", authenticated(maxImportBodyBytes)...)
	backupUploadLimit := d.Config.BackupMaxUploadBytes
	if backupUploadLimit <= 0 {
		backupUploadLimit = 2 << 30
	}
	backupUploads := r.Group("/api", authenticated(backupUploadLimit+(1<<20))...)
	userSvc := user.NewService(users, user.WithStorage(d.Storage))
	userOpts := userapi.Options{RBAC: d.RBAC}
	if d.Game != nil {
		userOpts.IdentityLookup = d.Game
	}
	userapi.RegisterAvatarRoutes(public, uploads, userSvc)
	if d.RBAC != nil {
		adminapi.RegisterUserImportRoute(uploads, d.RBAC, userimport.NewService(d.DB))
		adminapi.RegisterRoutes(api, d.RBAC, adminapi.Options{
			Game: d.Game, Rating: d.Rating, AI: d.AISettings,
			AIPlatform: d.AIPlatform, Agent: d.Agent,
		})
	}
	jobsapi.RegisterRoutes(api, d.Jobs, d.Difficulty, d.RBAC)
	if d.Game != nil {
		gamificationapi.RegisterRoutes(api, uploads, d.Game,
			gamificationapi.Options{RBAC: d.RBAC, Companion: d.Companion})
	}
	if d.Companion != nil {
		companionapi.RegisterRoutes(api, d.Companion)
	}
	if d.Rating != nil {
		if d.Game != nil {
			ratingapi.RegisterRoutes(api, d.Rating, ratingapi.Options{IdentityLookup: d.Game})
		} else {
			ratingapi.RegisterRoutes(api, d.Rating)
		}
	}
	if d.Activity != nil {
		if d.Game != nil {
			activityapi.RegisterRoutes(api, d.Activity,
				activityapi.Options{IdentityLookup: d.Game, Companion: d.Companion})
		} else {
			activityapi.RegisterRoutes(api, d.Activity, activityapi.Options{Companion: d.Companion})
		}
	}
	if d.Learning != nil {
		learningapi.RegisterRoutes(api, d.Learning, learningapi.Options{RBAC: d.RBAC})
	}
	if d.Certificate != nil {
		certificateapi.RegisterRoutes(public, api, uploads, d.Certificate, certificateapi.Options{RBAC: d.RBAC})
	}
	siteSvc := site.NewService(site.NewGormRepository(d.DB), d.Storage)
	if d.Companion != nil {
		d.Companion.SetCompanionNameProvider(siteCompanionName{svc: siteSvc})
	}
	siteapi.RegisterRoutes(public, api, uploads, siteSvc, d.RBAC)
	if d.Mail != nil {
		mailapi.RegisterRoutes(api, d.Mail, d.RBAC)
	}
	contestRepo := contest.NewGormRepository(d.DB)
	problemRepo := problem.NewGormRepository(d.DB)
	if d.AI != nil {
		aiapi.RegisterRoutes(api, d.AI, aiapi.Options{
			RBAC: d.RBAC, Settings: d.AISettings,
			ContestAccess: contestRepo, Problems: problemRepo,
		})
	}
	contestSvc := contest.NewService(contestRepo)
	contestOpts := contestapi.Options{RBAC: d.RBAC}
	if d.Game != nil {
		contestOpts.GameHook = d.Game
		contestOpts.IdentityLookup = d.Game
	}
	if d.Activity != nil {
		contestOpts.ActivityHook = d.Activity
	}
	contestapi.RegisterRoutes(api, contestSvc, contestOpts)
	if d.Similarity != nil {
		similarityapi.RegisterRoutes(api, d.Similarity, similarityapi.Options{RBAC: d.RBAC})
	}
	submissionRepo := submission.NewGormRepository(d.DB)
	var problemSvc *problem.Service
	if d.Learning != nil {
		problemSvc = problem.NewService(problemRepo, d.Storage, problem.WithLearningGuard(d.Learning))
	} else {
		problemSvc = problem.NewService(problemRepo, d.Storage)
	}
	socialSvc := social.NewService(d.DB, d.Activity)
	socialapi.RegisterRoutes(api, socialSvc)
	problemOpts := problemapi.Options{
		RBAC:        d.RBAC,
		Solutions:   problem.NewOfficialSolutionService(d.DB, problemRepo),
		Discussions: problem.NewDiscussionService(d.DB, problemRepo),
		Social:      socialSvc,
		TestcaseGen: d.TestcaseGen,
		Difficulty:  d.Difficulty,
		AI:          d.DifficultyAI,
		AISettings:  d.DifficultyAISettings,
	}
	if d.Game != nil {
		problemOpts.IdentityLookup = d.Game
	}
	problemapi.RegisterRoutes(api, problemJSON, uploads, imports,
		problemSvc, submissionRepo, contestRepo, problemOpts)
	submissionSvc := submission.NewService(
		submissionRepo,
		problemRepo,
		contestRepo,
		d.Enqueuer,
		submission.NewRedisLimiter(d.Redis, d.Config.SubmitCooldown),
		submissionevents.NewRedisPublisher(d.Redis),
	)
	// Profile activity needs submissionSvc, so user routes are mounted here
	// rather than next to the other user services above.
	userOpts.Activity = submissionSvc
	userapi.RegisterRoutes(api, userSvc, userOpts)
	streamer := submissionevents.NewRedisStreamer(d.Redis)
	submissionOpts := submissionapi.Options{RBAC: d.RBAC, Companion: d.Companion}
	if d.Game != nil {
		submissionOpts.IdentityLookup = d.Game
	}
	if d.Activity != nil {
		submissionOpts.ActivityHook = d.Activity
	}
	submissionapi.RegisterRoutes(api, submissionSvc, streamer, submissionOpts)
	if d.Backup != nil {
		backupapi.RegisterRoutes(api, backupUploads, d.Backup, d.RBAC)
	}
	return r
}

// requestLog records method, path, status and latency only; request bodies
// may contain passwords and tokens and are never logged.
func requestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds())
	}
}

type dbPinger struct{ db *gorm.DB }

func (p dbPinger) Ping(ctx context.Context) error {
	sqlDB, err := p.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

type redisPinger struct{ rdb *redis.Client }

func (p redisPinger) Ping(ctx context.Context) error { return p.rdb.Ping(ctx).Err() }

type siteCompanionName struct{ svc *site.Service }

func (p siteCompanionName) CompanionName(ctx context.Context) (string, error) {
	view, err := p.svc.Public(ctx)
	if err != nil {
		return "Koj", err
	}
	if name := strings.TrimSpace(view.Companion.Name); name != "" {
		return name, nil
	}
	return "Koj", nil
}
