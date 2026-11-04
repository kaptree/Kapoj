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

	"github.com/kaptree/kapoj/apps/server/internal/config"
	"github.com/kaptree/kapoj/apps/server/internal/db"
	"github.com/kaptree/kapoj/apps/server/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/server/internal/gamification"
	"github.com/kaptree/kapoj/apps/server/internal/judgequeue"
	"github.com/kaptree/kapoj/apps/server/internal/router"
	"github.com/kaptree/kapoj/apps/server/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
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
	store, err := storage.NewLocal(cfg.StorageDir)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() { _ = rdb.Close() }()
	queue := judgequeue.NewClient(cfg.RedisAddr)
	defer func() { _ = queue.Close() }()
	gamePublisher := gamequeue.NewPublisher(cfg.RedisAddr)
	defer func() { _ = gamePublisher.Close() }()
	gameSvc := gamification.NewService(gamification.NewRepository(g), gamePublisher)

	gameServer := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency:     cfg.GameConcurrency,
			Queues:          map[string]int{gamequeue.QueueName: 1},
			ShutdownTimeout: 10 * time.Second,
		},
	)
	gameMux := asynq.NewServeMux()
	gamequeue.Register(gameMux, gameSvc)
	if err := gameServer.Start(gameMux); err != nil {
		return err
	}
	defer gameServer.Shutdown()
	slog.Info("kapoj game worker started",
		"queue", gamequeue.QueueName, "concurrency", cfg.GameConcurrency)

	gin.SetMode(gin.ReleaseMode)
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: router.New(router.Deps{
			DB: g, Redis: rdb, Storage: store, Config: cfg, Enqueuer: queue, Game: gameSvc,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	// SSE handlers only exit when their request context is canceled; cancel
	// them before Shutdown waits for active handlers.
	srv.BaseContext = func(net.Listener) context.Context { return baseCtx }
	srv.RegisterOnShutdown(cancelBase)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("kapoj server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
