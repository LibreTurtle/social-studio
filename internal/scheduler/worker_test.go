package scheduler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LibreTurtle/social-studio/internal/publishing"
)

type queueFake struct {
	mu         sync.Mutex
	claims     []publishing.Claim
	recoveries int
	claimLimit int
	recoverErr error
	claimErr   error
	stopRun    context.CancelFunc
}

func (queue *queueFake) RecoverStale(context.Context, time.Duration) (publishing.Recovery, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	queue.recoveries++
	if queue.stopRun != nil {
		queue.stopRun()
	}
	return publishing.Recovery{}, queue.recoverErr
}

func (queue *queueFake) ClaimDue(_ context.Context, limit int) ([]publishing.Claim, error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	queue.claimLimit = limit
	if queue.claimErr != nil {
		return nil, queue.claimErr
	}
	claims := queue.claims
	queue.claims = nil
	return claims, nil
}

type executorFake struct {
	mu     sync.Mutex
	claims []publishing.Claim
	err    error
	wait   <-chan struct{}
}

func (executor *executorFake) PublishClaim(_ context.Context, claim publishing.Claim) (publishing.Attempt, error) {
	if executor.wait != nil {
		<-executor.wait
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.claims = append(executor.claims, claim)
	return publishing.Attempt{}, executor.err
}

func TestRunOnceRecoversAndPublishesClaimedDueSchedules(t *testing.T) {
	claims := []publishing.Claim{
		{Schedule: publishing.Schedule{ID: "schedule-1"}},
		{Schedule: publishing.Schedule{ID: "schedule-2"}},
	}
	queue := &queueFake{claims: claims}
	executor := &executorFake{}
	worker, err := NewWorker(queue, executor, Options{PollInterval: time.Second, ClaimTimeout: time.Minute, Concurrency: 2}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.recoveries != 1 || queue.claimLimit != 2 || len(executor.claims) != 2 {
		t.Fatalf("recoveries=%d claim_limit=%d published=%d", queue.recoveries, queue.claimLimit, len(executor.claims))
	}
}

func TestRunOnceContinuesAfterOneScheduleFailure(t *testing.T) {
	queue := &queueFake{claims: []publishing.Claim{{Schedule: publishing.Schedule{ID: "schedule-1"}}, {Schedule: publishing.Schedule{ID: "schedule-2"}}}}
	executor := &executorFake{err: errors.New("provider unavailable")}
	worker, err := NewWorker(queue, executor, Options{PollInterval: time.Second, ClaimTimeout: time.Minute, Concurrency: 2}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(executor.claims) != 2 {
		t.Fatalf("published claims=%d, want both claims attempted", len(executor.claims))
	}
}

func TestRunOnceLogsFailureCategoryWithoutProviderErrorDetails(t *testing.T) {
	var logs bytes.Buffer
	queue := &queueFake{claims: []publishing.Claim{{Schedule: publishing.Schedule{ID: "schedule-1"}, Platform: "X"}}}
	executor := &executorFake{err: errors.New("provider returned secret response body")}
	worker, err := NewWorker(queue, executor, Options{PollInterval: time.Second, ClaimTimeout: time.Minute, Concurrency: 1}, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), `"error_category":"publish_failed"`) || strings.Contains(logs.String(), "secret response body") {
		t.Fatalf("worker logged raw provider error or omitted category: %s", logs.String())
	}
}

func TestRunStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	queue := &queueFake{stopRun: cancel}
	worker, err := NewWorker(queue, &executorFake{}, Options{PollInterval: 4 * time.Minute, ClaimTimeout: time.Minute, Concurrency: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if queue.recoveries != 1 {
		t.Fatalf("recovery calls=%d, want initial immediate pass", queue.recoveries)
	}
}

func TestNewWorkerRejectsUnsafeOptions(t *testing.T) {
	_, err := NewWorker(&queueFake{}, &executorFake{}, Options{PollInterval: 0, ClaimTimeout: time.Minute, Concurrency: 1}, slog.Default())
	if err == nil {
		t.Fatal("NewWorker() accepted a zero poll interval")
	}
	_, err = NewWorker(&queueFake{}, &executorFake{}, Options{PollInterval: time.Second, ClaimTimeout: time.Second, Concurrency: 0}, slog.Default())
	if err == nil {
		t.Fatal("NewWorker() accepted zero concurrency")
	}
	_, err = NewWorker(&queueFake{}, &executorFake{}, Options{PollInterval: time.Second, ClaimTimeout: time.Minute, Concurrency: maximumConcurrency + 1}, slog.Default())
	if err == nil {
		t.Fatal("NewWorker() accepted concurrency above its resource bound")
	}
}
