package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/kaptree/kapoj/apps/judge/internal/config"
	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/pipeline"
	"github.com/kaptree/kapoj/apps/judge/internal/queue"
	"github.com/kaptree/kapoj/apps/judge/internal/reconcile"
	"github.com/kaptree/kapoj/apps/judge/internal/sandbox"
	"github.com/kaptree/kapoj/apps/judge/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("judge stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, err := os.Stat(cfg.StorageDir); err != nil {
		return fmt.Errorf("KAPOJ_STORAGE_DIR %q is not readable: %w", cfg.StorageDir, err)
	}
	ctx := context.Background()
	repo, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer repo.Close()

	images := make(map[string]string, len(cfg.Images))
	for language, image := range cfg.Images {
		images[string(language)] = image
	}
	sb := sandbox.NewDocker(cfg.DockerBin, images)
	sb.Sweep(ctx)
	pipe := pipeline.New(repo, cfg.StorageDir, sb)
	judgeWorker := worker.New(pipe)
	queueClient := queue.NewClient(cfg.RedisAddr)
	defer func() { _ = queueClient.Close() }()

	reconcileCtx, stopReconcile := context.WithCancel(context.Background())
	defer stopReconcile()
	go reconcile.Run(reconcileCtx, repo, queueClient, time.Minute)

	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency:     cfg.Concurrency,
			Queues:          map[string]int{queue.QueueName: 1},
			ShutdownTimeout: 2 * time.Minute,
		},
	)
	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypeSubmission, judgeWorker.Handle)
	if err := server.Start(mux); err != nil {
		return err
	}
	slog.Info("kapoj judge started", "queue", queue.QueueName, "concurrency", cfg.Concurrency)

	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-stop.Done()
	slog.Info("kapoj judge shutting down")
	stopReconcile()
	server.Shutdown()
	sb.Sweep(context.Background())
	return nil
}
