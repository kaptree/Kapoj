package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"github.com/kaptree/kapoj/apps/judge/internal/config"
	"github.com/kaptree/kapoj/apps/judge/internal/db"
	"github.com/kaptree/kapoj/apps/judge/internal/pipeline"
	"github.com/kaptree/kapoj/apps/judge/internal/queue"
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
	pipe := pipeline.New(repo, cfg.StorageDir, sb)
	judgeWorker := worker.New(pipe)

	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency: cfg.Concurrency,
			Queues:      map[string]int{queue.QueueName: 1},
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
	server.Shutdown()
	return nil
}
