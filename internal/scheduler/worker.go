package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"uuid"

	"github.com/LibreTurtle/social-studio/internal/publishing"
)

const maximumConcurrency = 128

const (
	minimumPollInterval = 100 * time.Millisecond
	maximumPollInterval = 5 * time.Minute
	minimumClaimTimeout = 10 * time.Second
	maximumClaimTimeout = 24 * time.Hour
)

type Queue interface {
	RecoverStale(context.Context, time.Duration) (publishing.Recovery, error)
	ClaimDue(context.Context, int) ([]publishing.Claim, error)
}

type Executor interface {
	PublishClaim(context.Context, publishing.Claim) (publishing.Attempt, error)
}

type Options struct {
	PollInterval time.Duration
	ClaimTimeout time.Duration
	Concurrency  int
}

type Worker struct {
	queue    Queue
	executor Executor
	options  Options
	logger   *slog.Logger
	workerID string
}

func NewWorker(queue Queue, executor Executor, options Options, logger *slog.Logger) (*Worker, error) {
	if queue == nil || executor == nil ||
		options.PollInterval < minimumPollInterval || options.PollInterval > maximumPollInterval ||
		options.ClaimTimeout < minimumClaimTimeout || options.ClaimTimeout > maximumClaimTimeout ||
		options.Concurrency < 1 || options.Concurrency > maximumConcurrency {
		return nil, errors.New("scheduler worker configuration is invalid")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{
		queue: queue, executor: executor, options: options, logger: logger,
		workerID: uuid.NewV4().String(),
	}, nil
}

func (worker *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(worker.options.PollInterval)
	defer ticker.Stop()
	for {
		if err := worker.RunOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			worker.logger.ErrorContext(ctx, "scheduler polling failed", "worker_id", worker.workerID, "error_category", "queue_unavailable")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (worker *Worker) RunOnce(ctx context.Context) error {
	recovery, err := worker.queue.RecoverStale(ctx, worker.options.ClaimTimeout)
	if err != nil {
		return fmt.Errorf("recover stale publishing claims: %w", err)
	}
	if recovery.Requeued > 0 || recovery.Unknown > 0 {
		worker.logger.WarnContext(ctx, "stale publishing claims recovered", "worker_id", worker.workerID, "requeued", recovery.Requeued, "outcome_unknown", recovery.Unknown)
	}
	if ctx.Err() != nil {
		return nil
	}
	claims, err := worker.queue.ClaimDue(ctx, worker.options.Concurrency)
	if err != nil {
		return fmt.Errorf("claim due publishing schedules: %w", err)
	}
	if len(claims) == 0 {
		return nil
	}
	worker.logger.InfoContext(ctx, "publishing schedules claimed", "worker_id", worker.workerID, "count", len(claims))
	var workers sync.WaitGroup
	workers.Add(len(claims))
	for _, claim := range claims {
		claim := claim
		go func() {
			defer workers.Done()
			if _, err := worker.executor.PublishClaim(ctx, claim); err != nil {
				category := "publish_failed"
				if errors.Is(err, publishing.ErrUncertain) {
					category = "provider_outcome_unknown"
				}
				worker.logger.ErrorContext(ctx, "scheduled publish did not complete", "worker_id", worker.workerID, "schedule_id", claim.Schedule.ID, "platform", claim.Platform, "error_category", category)
			}
		}()
	}
	workers.Wait()
	return nil
}
