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
	"github.com/kaptree/kapoj/apps/judge/internal/events"
	"github.com/kaptree/kapoj/apps/judge/internal/gamequeue"
	"github.com/kaptree/kapoj/apps/judge/internal/generation"
	"github.com/kaptree/kapoj/apps/judge/internal/pipeline"
	"github.com/kaptree/kapoj/apps/judge/internal/queue"
	"github.com/kaptree/kapoj/apps/judge/internal/reconcile"
	"github.com/kaptree/kapoj/apps/judge/internal/sandbox"
	"github.com/kaptree/kapoj/apps/judge/internal/storage"
	"github.com/kaptree/kapoj/apps/judge/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("judge stopped", "err", err)
		os.Exit(1)
	}
}

func ensureBucket(ctx context.Context, store *storage.MinIO) error {
	var lastErr error
	for attempt := 0; attempt < 60; attempt++ {
		if lastErr = store.EnsureBucket(ctx); lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w (last error: %v)", ctx.Err(), lastErr)
		case <-time.After(time.Second):
		}
	}
	return lastErr
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var store storage.Store
	if cfg.StorageBackend == "minio" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		minioStore, err := storage.NewMinIO(storage.MinIOOptions{
			Endpoint: cfg.MinIOEndpoint, AccessKey: cfg.MinIOAccessKey,
			SecretKey: cfg.MinIOSecretKey, Bucket: cfg.MinIOBucket,
			Region: cfg.MinIORegion, UseSSL: cfg.MinIOUseSSL,
			AutoCreateBucket: cfg.MinIOAutoCreateBucket,
		})
		if err != nil {
			return err
		}
		if err := ensureBucket(ctx, minioStore); err != nil {
			return err
		}
		store = minioStore
		slog.Info("judge storage backend ready", "backend", "minio", "bucket", cfg.MinIOBucket)
	} else {
		if _, err := os.Stat(cfg.StorageDir); err != nil {
			return fmt.Errorf("KAPOJ_STORAGE_DIR %q is not readable: %w", cfg.StorageDir, err)
		}
		local, err := storage.NewLocal(cfg.StorageDir)
		if err != nil {
			return err
		}
		store = local
		slog.Info("judge storage backend ready", "backend", "local", "dir", cfg.StorageDir)
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
	publisher := events.NewRedisPublisher(cfg.RedisAddr)
	defer func() { _ = publisher.Close() }()
	gamePublisher := gamequeue.NewPublisher(cfg.RedisAddr)
	defer func() { _ = gamePublisher.Close() }()
	pipe := pipeline.New(repo, store, sb, publisher, gamePublisher)
	judgeWorker := worker.New(pipe)
	gen := generation.New(repo, store, sb)
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
	mux.HandleFunc(queue.TypeTestcaseGeneration, gen.Handle)
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
