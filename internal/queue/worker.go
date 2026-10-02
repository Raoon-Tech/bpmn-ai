package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// JobHandler processes a single job. Returns nil on success, error on failure.
type JobHandler func(ctx context.Context, job *store.JobRecord) error

// WorkerPool polls for pending jobs and processes them concurrently.
type WorkerPool struct {
	store       JobStore
	handler     JobHandler
	retry       *RetryPolicy
	dlq         *DeadLetterQueue
	concurrency int
	pollInterval time.Duration
	stopCh      chan struct{}
	wg         sync.WaitGroup
	dispatcher *observability.Dispatcher
}

// WorkerPoolConfig holds worker pool configuration.
type WorkerPoolConfig struct {
	Concurrency  int
	PollInterval time.Duration
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(s JobStore, handler JobHandler, retry *RetryPolicy, dlq *DeadLetterQueue, cfg WorkerPoolConfig) *WorkerPool {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.PollInterval < 1 {
		cfg.PollInterval = 5 * time.Second
	}
	if retry == nil {
		retry = DefaultRetryPolicy()
	}
	return &WorkerPool{
		store:        s,
		handler:      handler,
		retry:        retry,
		dlq:          dlq,
		concurrency:  cfg.Concurrency,
		pollInterval: cfg.PollInterval,
		stopCh:       make(chan struct{}),
	}
}

// Start begins polling for and processing jobs.
func (wp *WorkerPool) Start(ctx context.Context) {
	for i := 0; i < wp.concurrency; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx, i)
	}
}

// Stop signals all workers to shut down and waits for them to finish.
func (wp *WorkerPool) Stop() {
	close(wp.stopCh)
	wp.wg.Wait()
}

func (wp *WorkerPool) worker(ctx context.Context, id int) {
	defer wp.wg.Done()

	ticker := time.NewTicker(wp.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-wp.stopCh:
			return
		case <-ticker.C:
			wp.processBatch(ctx)
		}
	}
}

func (wp *WorkerPool) processBatch(ctx context.Context) {
	jobs, err := wp.store.GetPendingJobs(ctx, wp.concurrency)
	if err != nil {
		return
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return
		case <-wp.stopCh:
			return
		default:
			wp.processJob(ctx, job)
		}
	}
}

func (wp *WorkerPool) processJob(ctx context.Context, job *store.JobRecord) {
	// Each call to UpdateJob stores a freshly allocated copy so the
	// pointer in the map is never mutated after insertion — concurrent
	// readers (GetPendingJobs / GetJob) never race with a writer.
	if wp.handler == nil {
		j := new(store.JobRecord)
		*j = *job
		j.Status = store.JobStatusCompleted
		now := time.Now()
		j.ExecutedAt = &now
		_ = wp.store.UpdateJob(ctx, j)
		return
	}

	// Phase 1: transition to Running with a fresh copy.
	running := new(store.JobRecord)
	*running = *job
	running.Status = store.JobStatusRunning
	now := time.Now()
	running.ExecutedAt = &now
	if err := wp.store.UpdateJob(ctx, running); err != nil {
		return
	}

	if err := wp.handler(ctx, running); err != nil {
		wp.failJob(ctx, running, err)
		return
	}

	// Phase 2: transition to Completed with a fresh copy.
	completed := new(store.JobRecord)
	*completed = *running
	completed.Status = store.JobStatusCompleted
	now = time.Now()
	completed.ExecutedAt = &now
	_ = wp.store.UpdateJob(ctx, completed)

	if wp.dispatcher != nil {
		wp.dispatcher.DispatchAsync(observability.Event{
			Type:      observability.EventJobCompleted,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"instance_id": completed.InstanceID,
				"job_id":      completed.ID,
				"job_type":    completed.Type,
			},
		})
	}
}

func (wp *WorkerPool) failJob(ctx context.Context, job *store.JobRecord, err error) {
	j := new(store.JobRecord)
	*j = *job
	j.ErrorMessage = err.Error()

	if wp.retry.ShouldRetry(j) {
		wp.retry.ApplyRetry(j, err.Error())
		_ = wp.store.UpdateJob(ctx, j)

		if wp.dispatcher != nil {
			wp.dispatcher.DispatchAsync(observability.Event{
				Type:      observability.EventJobFailed,
				Timestamp: time.Now(),
				Payload: map[string]any{
					"instance_id":  j.InstanceID,
					"job_id":       j.ID,
					"job_type":     j.Type,
					"error":        err.Error(),
					"retry_count":  j.RetryCount,
				},
			})
		}
		return
	}

	if wp.dlq != nil {
		j.Status = store.JobStatusDead
		_ = wp.dlq.Add(ctx, j, err.Error())
		return
	}

	j.Status = store.JobStatusDead
	_ = wp.store.UpdateJob(ctx, j)
}

// WithDispatcher sets the event dispatcher for the worker pool.
func (wp *WorkerPool) WithDispatcher(d *observability.Dispatcher) *WorkerPool {
	wp.dispatcher = d
	return wp
}

// WithHandler sets the job handler for the worker pool.
func (wp *WorkerPool) WithHandler(handler JobHandler) *WorkerPool {
	wp.handler = handler
	return wp
}

// Enqueue creates a new job and adds it to the queue.
func (wp *WorkerPool) Enqueue(ctx context.Context, job *store.JobRecord) error {
	if job.ID == "" {
		job.ID = fmt.Sprintf("job-%s", job.InstanceID)
	}
	if job.Status == "" {
		job.Status = store.JobStatusPending
	}
	if job.ScheduledAt.IsZero() {
		job.ScheduledAt = time.Now()
	}
	if job.MaxRetries == 0 {
		job.MaxRetries = wp.retry.MaxRetries
	}
	return wp.store.CreateJob(ctx, job)
}
