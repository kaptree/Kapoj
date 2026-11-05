package router

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/contestapi"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/gamificationapi"
	"github.com/kaptree/kapoj/apps/server/internal/health"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/problemapi"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/ratingapi"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
	"github.com/kaptree/kapoj/apps/server/internal/submission"
	"github.com/kaptree/kapoj/apps/server/internal/submissionapi"
	"github.com/kaptree/kapoj/apps/server/internal/submissionevents"
	"github.com/kaptree/kapoj/apps/server/internal/user"
	"github.com/kaptree/kapoj/apps/server/internal/userapi"
)

const (
	maxBodyBytes        = 1 << 20  // JSON endpoints
	maxProblemJSONBytes = 4 << 20  // problem create/update: statements and samples
	maxUploadBytes      = 40 << 20 // file upload endpoints: two 16 MiB files plus form fields
)

type Deps struct {
	DB       *gorm.DB
	Redis    *redis.Client
	Storage  storage.Storage
	Config   *config.Config
	Enqueuer submission.Enqueuer
	Game     *gamification.Service
	Rating   *rating.Service
	Activity *activity.Service
}

func New(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(requestLog(), gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic recovered", "path", c.Request.URL.Path, "panic", recovered)
		httpx.Fail(c, httpx.ErrInternal)
	}))
	r.NoRoute(func(c *gin.Context) { httpx.Fail(c, httpx.ErrNotFound) })

	r.GET("/health", health.Handler(dbPinger{d.DB}, redisPinger{d.Redis}))

	users := user.NewGormRepository(d.DB)
	issuer := auth.NewTokenIssuer(d.Config.JWTSecret, d.Config.AccessTokenTTL)
	authSvc := auth.NewService(users, issuer,
		auth.NewRefreshStore(d.Redis, d.Config.RefreshTokenTTL),
		auth.NewLoginLimiter(d.Redis))

	var loginHooks []auth.LoginHook
	if d.Game != nil {
		loginHooks = append(loginHooks, d.Game)
	}

	// Auth endpoints identify the caller by the request body, not the bearer
	// token, so an expired access token must not block logout or refresh.
	auth.RegisterRoutes(r.Group("/api", middleware.MaxBody(maxBodyBytes)), authSvc, loginHooks...)

	api := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(maxBodyBytes))
	problemJSON := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(maxProblemJSONBytes))
	uploads := r.Group("/api", middleware.Authenticate(issuer), middleware.MaxBody(maxUploadBytes))
	userSvc := user.NewService(users)
	if d.Game != nil {
		userapi.RegisterRoutes(api, userSvc, userapi.Options{IdentityLookup: d.Game})
	} else {
		userapi.RegisterRoutes(api, userSvc)
	}
	if d.Game != nil {
		gamificationapi.RegisterRoutes(api, uploads, d.Game)
	}
	if d.Rating != nil {
		if d.Game != nil {
			ratingapi.RegisterRoutes(api, d.Rating, ratingapi.Options{IdentityLookup: d.Game})
		} else {
			ratingapi.RegisterRoutes(api, d.Rating)
		}
	}
	contestRepo := contest.NewGormRepository(d.DB)
	contestSvc := contest.NewService(contestRepo)
	contestOpts := contestapi.Options{}
	if d.Game != nil {
		contestOpts.GameHook = d.Game
		contestOpts.IdentityLookup = d.Game
	}
	if d.Activity != nil {
		contestOpts.ActivityHook = d.Activity
	}
	contestapi.RegisterRoutes(api, contestSvc, contestOpts)
	problemRepo := problem.NewGormRepository(d.DB)
	submissionRepo := submission.NewGormRepository(d.DB)
	problemapi.RegisterRoutes(api, problemJSON, uploads,
		problem.NewService(problemRepo, d.Storage), submissionRepo, contestRepo)
	submissionSvc := submission.NewService(
		submissionRepo,
		problemRepo,
		contestRepo,
		d.Enqueuer,
		submission.NewRedisLimiter(d.Redis, d.Config.SubmitCooldown),
		submissionevents.NewRedisPublisher(d.Redis),
	)
	streamer := submissionevents.NewRedisStreamer(d.Redis)
	submissionOpts := submissionapi.Options{}
	if d.Game != nil {
		submissionOpts.IdentityLookup = d.Game
	}
	if d.Activity != nil {
		submissionOpts.ActivityHook = d.Activity
	}
	submissionapi.RegisterRoutes(api, submissionSvc, streamer, submissionOpts)
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
