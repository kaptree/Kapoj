package router

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/kaptree/kapoj/apps/server/internal/auth"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/health"
	"github.com/kaptree/kapoj/apps/server/internal/httpx"
	"github.com/kaptree/kapoj/apps/server/internal/middleware"
	"github.com/kaptree/kapoj/apps/server/internal/user"
	"github.com/kaptree/kapoj/apps/server/internal/userapi"
)

const maxBodyBytes = 1 << 20

type Deps struct {
	DB     *gorm.DB
	Redis  *redis.Client
	Config *config.Config
}

func New(d Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), requestLog(), limitBody(maxBodyBytes))
	r.NoRoute(func(c *gin.Context) { httpx.Fail(c, httpx.ErrNotFound) })

	r.GET("/health", health.Handler(dbPinger{d.DB}, redisPinger{d.Redis}))

	users := user.NewGormRepository(d.DB)
	issuer := auth.NewTokenIssuer(d.Config.JWTSecret, d.Config.AccessTokenTTL)
	authSvc := auth.NewService(users, issuer,
		auth.NewRefreshStore(d.Redis, d.Config.RefreshTokenTTL),
		auth.NewLoginLimiter(d.Redis))

	api := r.Group("/api", middleware.Authenticate(issuer))
	auth.RegisterRoutes(api, authSvc)
	userapi.RegisterRoutes(api, user.NewService(users))
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

func limitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
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
