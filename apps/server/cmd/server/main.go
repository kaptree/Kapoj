package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/kaptree/kapoj/apps/server/internal/activity"
	"github.com/kaptree/kapoj/apps/server/internal/ai"
	"github.com/kaptree/kapoj/apps/server/internal/backup"
	"github.com/kaptree/kapoj/apps/server/internal/certificate"
	"github.com/kaptree/kapoj/apps/server/internal/companion"
	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/contest"
	"github.com/kaptree/kapoj/apps/server/internal/db"
	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/judgequeue"
	"github.com/kaptree/kapoj/apps/server/internal/learning"
	"github.com/kaptree/kapoj/apps/server/internal/mail"
	"github.com/kaptree/kapoj/apps/server/internal/problem"
	"github.com/kaptree/kapoj/apps/server/internal/rating"
	"github.com/kaptree/kapoj/apps/server/internal/rbac"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/secret"
	"github.com/kaptree/kapoj/apps/server/internal/similarity"
	"github.com/kaptree/kapoj/apps/server/internal/similarityqueue"
	"github.com/kaptree/kapoj/apps/server/internal/storageaudit"
	"github.com/kaptree/kapoj/apps/server/internal/testcasegen"
)

var errRestart = errors.New("restart requested")

func main() {
	if len(os.Args) > 1 && os.Args[1] == "migrate-storage" {
		if err := runMigrateStorage(); err != nil {
			slog.Error("storage migration failed", "err", err)
			os.Exit(1)
		}
		return
	}
	err := run()
	if errors.Is(err, errRestart) {
		executable, execErr := os.Executable()
		if execErr == nil {
			execErr = syscall.Exec(executable, os.Args, os.Environ())
		}
		slog.Error("server restart failed", "err", execErr)
		os.Exit(1)
	}
	if err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// runSimilaritySweeper periodically fails queued/running similarity runs that
// have been stuck for longer than the cutoff, so a crashed worker cannot block
// a problem's active-run slot forever. It runs until ctx is canceled.
func runSimilaritySweeper(ctx context.Context, repo similarity.Repository) {
	const (
		interval = time.Minute
		cutoff   = 15 * time.Minute
	)
	sweep := func() {
		sweepCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := repo.MarkStaleRunsFailed(sweepCtx, time.Now().Add(-cutoff)); err != nil && ctx.Err() == nil {
			slog.Warn("similarity: mark stale runs failed", "err", err)
		}
	}
	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("KAPOJ_DATABASE_URL is required")
	}

	g, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if err := db.Migrate(g); err != nil {
		return err
	}
	if err := rbac.Seed(context.Background(), g); err != nil {
		return err
	}
	if err := ai.SeedProviders(context.Background(), g); err != nil {
		return err
	}
	if err := certificate.SeedTypes(context.Background(), g); err != nil {
		return err
	}
	store, files, packages, err := openStorage(cfg)
	if err != nil {
		return err
	}
	if missing, err := storageaudit.Scan(context.Background(), g, files); err != nil {
		slog.Warn("storage: integrity scan failed", "err", err)
	} else if len(missing) > 0 {
		slog.Warn("storage: referenced files are missing", "count", len(missing))
		for index, item := range missing {
			if index >= 50 {
				break
			}
			slog.Warn("storage: missing file",
				"kind", item.Kind, "id", item.ID, "key", item.Key,
			)
		}
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() { _ = rdb.Close() }()
	activitySvc := activity.NewService(g, rdb)
	if err := activitySvc.BackfillSharedItems(context.Background()); err != nil {
		slog.Warn("activity: backfill shared items failed", "err", err)
	}
	queue := judgequeue.NewClient(cfg.RedisAddr)
	defer func() { _ = queue.Close() }()
	gamePublisher := gamequeue.NewPublisher(cfg.RedisAddr)
	defer func() { _ = gamePublisher.Close() }()
	companionSvc := companion.NewService(companion.NewGormRepository(g))
	gameSvc := gamification.NewServiceWithOptions(
		gamification.NewRepository(g),
		gamification.ServiceOptions{
			MaxEquippedBadges: cfg.MaxEquippedBadges,
			Storage:           store,
			BondRecorder:      companionSvc,
		},
		gamePublisher,
	)
	ratingSvc := rating.NewService(g, contest.NewService(contest.NewGormRepository(g)), rdb, rating.Options{
		JudgeGrace:   cfg.RatingJudgeGrace,
		ScanInterval: cfg.RatingScanInterval,
	})
	learningSvc := learning.NewService(learning.NewRepository(g))
	rbacSvc := rbac.NewService(g)
	backupSvc, err := backup.NewService(g, cfg.DatabaseURL, backup.Options{
		Files:                files,
		Packages:             packages,
		WorkDir:              os.TempDir(),
		MaxUploadBytes:       cfg.BackupMaxUploadBytes,
		MaxUncompressedBytes: cfg.BackupMaxUncompressedBytes,
		MaxEntries:           cfg.BackupMaxEntries,
	})
	if err != nil {
		return err
	}
	if err := backupSvc.Ensure(context.Background()); err != nil {
		return err
	}
	backupQueue := backup.NewClient(cfg.RedisAddr)
	defer func() { _ = backupQueue.Close() }()
	backupSvc.SetQueue(backupQueue)
	certSvc := certificate.NewService(certificate.NewGormRepository(g), store)
	mailKeys, err := secret.NewKeyManager(cfg.MailEncryptionKey, cfg.MailKeyFile)
	if err != nil {
		return err
	}
	mailSvc := mail.NewService(mail.NewGormRepository(g), mailKeys, mail.WithTimeout(cfg.MailTimeout))

	// KeyManager loads the AI encryption key from the environment (priority) or
	// a local file. An invalid env key fails startup; a missing/invalid file
	// degrades to "not configured" so the admin UI can generate one later.
	keyMgr, err := ai.NewKeyManager(cfg.AIEncryptionKey, cfg.AIKeyFile)
	if err != nil {
		return err
	}
	aiSvc := ai.NewService(g, keyMgr,
		ai.NewRedisLimiter(rdb, cfg.AICooldown),
		ai.NewOpenAIClient(cfg.AIRequestTimeout))
	aiSettings := ai.NewSettingsService(g, keyMgr, cfg.AIKeyFile)
	testcaseGenSvc := testcasegen.NewService(
		testcasegen.NewGormRepository(g),
		problem.NewGormRepository(g),
		store,
		aiSvc,
		queue,
	)
	similarityQueue := similarityqueue.NewClient(cfg.RedisAddr)
	defer func() { _ = similarityQueue.Close() }()
	similarityRepo := similarity.NewGormRepository(g)
	similaritySvc := similarity.NewService(
		similarityRepo,
		contest.NewGormRepository(g),
		similarity.NewWinnowingEngine(),
		similarityQueue,
	)

	// 启动时核对密钥文件与数据库指纹，恢复“写文件成功但重加密事务未提交”
	// 这一崩溃窗口；不一致且无法恢复时拒绝启动。
	if err := ai.ReconcileKeyFile(context.Background(), g, keyMgr, cfg.AIKeyFile); err != nil {
		return err
	}

	gameServer := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency:     cfg.GameConcurrency,
			Queues:          map[string]int{gamequeue.QueueName: 1},
			ShutdownTimeout: 10 * time.Second,
		},
	)
	gameMux := asynq.NewServeMux()
	gameHandlers := []gamequeue.Handler{gameSvc}
	if learningSvc != nil {
		gameHandlers = append(gameHandlers, learningSvc)
	}
	if companionSvc != nil {
		gameHandlers = append(gameHandlers, companionSvc)
	}
	gamequeue.Register(gameMux, gamequeue.NewChain(gameHandlers...))
	if err := gameServer.Start(gameMux); err != nil {
		return err
	}
	defer gameServer.Shutdown()
	slog.Info("kapoj game worker started",
		"queue", gamequeue.QueueName, "concurrency", cfg.GameConcurrency)

	backupServer := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency:     1,
			Queues:          map[string]int{backup.QueueName: 1},
			ShutdownTimeout: 10 * time.Second,
		},
	)
	backupMux := asynq.NewServeMux()
	backup.Register(backupMux, backupSvc)
	if err := backupServer.Start(backupMux); err != nil {
		return err
	}
	defer backupServer.Shutdown()
	slog.Info("kapoj backup worker started", "queue", backup.QueueName)

	similarityServer := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency:     1,
			Queues:          map[string]int{similarityqueue.QueueName: 1},
			ShutdownTimeout: 10 * time.Second,
		},
	)
	similarityMux := asynq.NewServeMux()
	similarityqueue.Register(similarityMux, similaritySvc)
	if err := similarityServer.Start(similarityMux); err != nil {
		return err
	}
	defer similarityServer.Shutdown()
	slog.Info("kapoj similarity worker started", "queue", similarityqueue.QueueName)

	gin.SetMode(gin.ReleaseMode)
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: router.New(router.Deps{
			DB: g, Redis: rdb, Storage: store, Config: cfg, Enqueuer: queue, Game: gameSvc,
			Rating: ratingSvc, Activity: activitySvc, Learning: learningSvc, RBAC: rbacSvc,
			AI: aiSvc, AISettings: aiSettings, TestcaseGen: testcaseGenSvc, Certificate: certSvc,
			Mail: mailSvc, Backup: backupSvc, Similarity: similaritySvc,
			Companion: companionSvc,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// SSE handlers only exit when their request context is canceled; cancel
	// them before Shutdown waits for active handlers.
	srv.BaseContext = func(net.Listener) context.Context { return baseCtx }
	srv.RegisterOnShutdown(cancelBase)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go ratingSvc.Run(ctx)
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	go backupSvc.RunScheduler(workCtx, 30*time.Second)
	go runSimilaritySweeper(workCtx, similarityRepo)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("kapoj server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	restart := false
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	case <-backupSvc.RestartRequested():
		restart = true
		cancelWork()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if sqlDB, err := g.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if restart {
		return errRestart
	}
	return nil
}
