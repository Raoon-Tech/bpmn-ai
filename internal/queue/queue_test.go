package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/memory"
)

func TestRetryPolicy_NextRetryAt(t *testing.T) {
	rp := &RetryPolicy{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  1 * time.Second,
		Jitter:    false,
	}

	tests := []struct {
		retryCount int
		wantDelay  time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{10, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			next := rp.NextRetryAt(tt.retryCount)
			delay := next.Sub(time.Now())
			tolerance := 50 * time.Millisecond
			if delay < tt.wantDelay-tolerance || delay > tt.wantDelay+tolerance {
				t.Errorf("retry %d: expected delay ~%v, got %v", tt.retryCount, tt.wantDelay, delay)
			}
		})
	}
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	rp := &RetryPolicy{MaxRetries: 3}

	job := &store.JobRecord{RetryCount: 0, MaxRetries: 5}
	if !rp.ShouldRetry(job) {
		t.Error("expected should retry at count 0")
	}

	job.RetryCount = 2
	if !rp.ShouldRetry(job) {
		t.Error("expected should retry at count 2")
	}

	job.RetryCount = 3
	if rp.ShouldRetry(job) {
		t.Error("expected should not retry at count 3")
	}
}

func TestRetryPolicy_ShouldRetry_ZeroMax(t *testing.T) {
	rp := &RetryPolicy{MaxRetries: 0}

	job := &store.JobRecord{RetryCount: 0, MaxRetries: 3}
	if !rp.ShouldRetry(job) {
		t.Error("expected should retry using job max retries")
	}

	job.RetryCount = 3
	if rp.ShouldRetry(job) {
		t.Error("expected should not retry at job max retries")
	}
}

func TestRetryPolicy_ApplyRetry(t *testing.T) {
	rp := &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		Jitter:     false,
	}

	job := &store.JobRecord{
		RetryCount: 0,
		Status:     store.JobStatusFailed,
	}

	before := time.Now()
	rp.ApplyRetry(job, "connection timeout")

	if job.RetryCount != 1 {
		t.Errorf("expected retry count 1, got %d", job.RetryCount)
	}
	if job.ErrorMessage != "connection timeout" {
		t.Errorf("expected error message 'connection timeout', got %s", job.ErrorMessage)
	}
	if job.Status != store.JobStatusPending {
		t.Errorf("expected status PENDING, got %s", job.Status)
	}
	if job.ScheduledAt.IsZero() {
		t.Error("expected scheduled at to be set")
	}
	expectedMin := before.Add(50 * time.Millisecond)
	if job.ScheduledAt.Before(expectedMin) {
		t.Errorf("expected scheduled at after %v, got %v", expectedMin, job.ScheduledAt)
	}
	if job.ExecutedAt != nil {
		t.Error("expected executed at to be nil after retry")
	}
}

func TestRetryError(t *testing.T) {
	inner := errors.New("timeout")
	re := &RetryError{
		Attempt:    2,
		MaxRetries: 3,
		NextRetry:  time.Now().Add(1 * time.Second),
		Err:        inner,
	}

	msg := re.Error()
	if msg == "" {
		t.Error("expected non-empty error message")
	}

	unwrapped := errors.Unwrap(re)
	if unwrapped != inner {
		t.Error("expected unwrap to return inner error")
	}
}

func TestDeadLetterQueue_Add(t *testing.T) {
	s := memory.NewStore()
	dlq := NewDeadLetterQueue(s, s)

	job := &store.JobRecord{
		ID:         "job-1",
		InstanceID: "inst-1",
		Type:       "serviceTask",
		Payload:    map[string]any{"url": "https://api.example.com"},
		Status:     store.JobStatusFailed,
		RetryCount: 3,
	}

	err := dlq.Add(context.Background(), job, "max retries exceeded")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.Status != store.JobStatusDead {
		t.Errorf("expected job status DEAD, got %s", job.Status)
	}

	dls, err := dlq.List(context.Background(), "inst-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dls) != 1 {
		t.Fatalf("expected 1 dead letter, got %d", len(dls))
	}

	dl := dls[0]
	if dl.JobID != "job-1" {
		t.Errorf("expected job ID job-1, got %s", dl.JobID)
	}
	if dl.ErrorMessage != "max retries exceeded" {
		t.Errorf("expected error message 'max retries exceeded', got %s", dl.ErrorMessage)
	}
	if dl.RetryCount != 3 {
		t.Errorf("expected retry count 3, got %d", dl.RetryCount)
	}
}

func TestDeadLetterQueue_ListAll(t *testing.T) {
	s := memory.NewStore()
	dlq := NewDeadLetterQueue(s, s)

	for i := 0; i < 5; i++ {
		job := &store.JobRecord{
			ID:         "job-dlq",
			InstanceID: "inst-all",
			Type:       "serviceTask",
			Status:     store.JobStatusFailed,
			RetryCount: 3,
		}
		_ = dlq.Add(context.Background(), job, "failed")
	}

	dls, err := dlq.ListAll(context.Background(), 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dls) > 3 {
		t.Errorf("expected at most 3 dead letters, got %d", len(dls))
	}
}

func TestDeadLetterQueue_Count(t *testing.T) {
	s := memory.NewStore()
	dlq := NewDeadLetterQueue(s, s)

	for i := 0; i < 3; i++ {
		job := &store.JobRecord{
			ID:         "job-count",
			InstanceID: "inst-count",
			Type:       "serviceTask",
			Status:     store.JobStatusFailed,
			RetryCount: 3,
		}
		_ = dlq.Add(context.Background(), job, "failed")
	}

	count, err := dlq.Count(context.Background(), "inst-count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestWorkerPool_Enqueue(t *testing.T) {
	s := memory.NewStore()
	retry := DefaultRetryPolicy()
	dlq := NewDeadLetterQueue(s, s)
	wp := NewWorkerPool(s, nil, retry, dlq, WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 100 * time.Millisecond,
	})

	job := &store.JobRecord{
		InstanceID: "inst-1",
		Type:       "serviceTask",
		Payload:    map[string]any{"action": "send_email"},
	}

	err := wp.Enqueue(context.Background(), job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.ID == "" {
		t.Error("expected job ID to be set")
	}
	if job.Status != store.JobStatusPending {
		t.Errorf("expected status PENDING, got %s", job.Status)
	}
	if job.MaxRetries != retry.MaxRetries {
		t.Errorf("expected max retries %d, got %d", retry.MaxRetries, job.MaxRetries)
	}

	pending, err := s.GetPendingJobs(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending job, got %d", len(pending))
	}
}

func TestWorkerPool_ProcessJob_Success(t *testing.T) {
	s := memory.NewStore()
	retry := DefaultRetryPolicy()
	dlq := NewDeadLetterQueue(s, s)

	handlerCalled := false
	handler := func(ctx context.Context, job *store.JobRecord) error {
		handlerCalled = true
		return nil
	}

	wp := NewWorkerPool(s, handler, retry, dlq, WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 50 * time.Millisecond,
	})

	job := &store.JobRecord{
		ID:           "job-success",
		InstanceID:   "inst-1",
		Type:         "serviceTask",
		Status:       store.JobStatusPending,
		ScheduledAt:  time.Now(),
		MaxRetries:   3,
	}
	_ = s.CreateJob(context.Background(), job)

	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()
	wp.Stop()

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	updated, err := s.GetJob(context.Background(), "job-success")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != store.JobStatusCompleted {
		t.Errorf("expected status COMPLETED, got %s", updated.Status)
	}
}

func TestWorkerPool_ProcessJob_Failure_Retry(t *testing.T) {
	s := memory.NewStore()
	retry := &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  10 * time.Millisecond,
		Jitter:     false,
	}
	dlq := NewDeadLetterQueue(s, s)

	attempts := 0
	handler := func(ctx context.Context, job *store.JobRecord) error {
		attempts++
		return errors.New("transient error")
	}

	wp := NewWorkerPool(s, handler, retry, dlq, WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 50 * time.Millisecond,
	})

	job := &store.JobRecord{
		ID:           "job-retry",
		InstanceID:   "inst-1",
		Type:         "serviceTask",
		Status:       store.JobStatusPending,
		ScheduledAt:  time.Now(),
		MaxRetries:   3,
		RetryCount:   0,
	}
	_ = s.CreateJob(context.Background(), job)

	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)

	time.Sleep(500 * time.Millisecond)
	cancel()
	wp.Stop()

	if attempts < 2 {
		t.Errorf("expected at least 2 attempts, got %d", attempts)
	}

	updated, err := s.GetJob(context.Background(), "job-retry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != store.JobStatusPending && updated.Status != store.JobStatusDead {
		t.Errorf("expected status PENDING or DEAD, got %s", updated.Status)
	}
}

func TestWorkerPool_ProcessJob_Failure_DeadLetter(t *testing.T) {
	s := memory.NewStore()
	retry := &RetryPolicy{
		MaxRetries: 1,
		BaseDelay:  10 * time.Millisecond,
		Jitter:     false,
	}
	dlq := NewDeadLetterQueue(s, s)

	handler := func(ctx context.Context, job *store.JobRecord) error {
		return errors.New("permanent error")
	}

	wp := NewWorkerPool(s, handler, retry, dlq, WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 50 * time.Millisecond,
	})

	job := &store.JobRecord{
		ID:           "job-dlq",
		InstanceID:   "inst-1",
		Type:         "serviceTask",
		Status:       store.JobStatusPending,
		ScheduledAt:  time.Now(),
		MaxRetries:   1,
		RetryCount:   0,
	}
	_ = s.CreateJob(context.Background(), job)

	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)

	time.Sleep(500 * time.Millisecond)
	cancel()
	wp.Stop()

	dls, err := dlq.List(context.Background(), "inst-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dls) == 0 {
		t.Error("expected job to be moved to dead letter queue")
	}
}

func TestWorkerPool_NoHandler(t *testing.T) {
	s := memory.NewStore()
	retry := DefaultRetryPolicy()
	dlq := NewDeadLetterQueue(s, s)

	wp := NewWorkerPool(s, nil, retry, dlq, WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 50 * time.Millisecond,
	})

	job := &store.JobRecord{
		ID:           "job-nohandler",
		InstanceID:   "inst-1",
		Type:         "serviceTask",
		Status:       store.JobStatusPending,
		ScheduledAt:  time.Now(),
		MaxRetries:   3,
	}
	_ = s.CreateJob(context.Background(), job)

	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()
	wp.Stop()

	updated, err := s.GetJob(context.Background(), "job-nohandler")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != store.JobStatusCompleted {
		t.Errorf("expected status COMPLETED with nil handler, got %s", updated.Status)
	}
}

func TestWorkerPool_ScheduledJobs(t *testing.T) {
	s := memory.NewStore()
	retry := DefaultRetryPolicy()
	dlq := NewDeadLetterQueue(s, s)

	handlerCalled := false
	handler := func(ctx context.Context, job *store.JobRecord) error {
		handlerCalled = true
		return nil
	}

	wp := NewWorkerPool(s, handler, retry, dlq, WorkerPoolConfig{
		Concurrency:  1,
		PollInterval: 50 * time.Millisecond,
	})

	job := &store.JobRecord{
		ID:           "job-future",
		InstanceID:   "inst-1",
		Type:         "serviceTask",
		Status:       store.JobStatusPending,
		ScheduledAt:  time.Now().Add(10 * time.Second),
		MaxRetries:   3,
	}
	_ = s.CreateJob(context.Background(), job)

	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()
	wp.Stop()

	if handlerCalled {
		t.Error("expected handler not to be called for future scheduled job")
	}

	pending, err := s.GetPendingJobs(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending jobs (future job should not be picked up), got %d", len(pending))
	}
}

func TestWorkerPool_ConcurrentWorkers(t *testing.T) {
	s := memory.NewStore()
	retry := DefaultRetryPolicy()
	dlq := NewDeadLetterQueue(s, s)

	processed := 0
	var mu sync.Mutex
	handler := func(ctx context.Context, job *store.JobRecord) error {
		mu.Lock()
		processed++
		mu.Unlock()
		return nil
	}

	wp := NewWorkerPool(s, handler, retry, dlq, WorkerPoolConfig{
		Concurrency:  3,
		PollInterval: 50 * time.Millisecond,
	})

	for i := 0; i < 5; i++ {
		job := &store.JobRecord{
			ID:           fmt.Sprintf("job-concurrent-%d", i),
			InstanceID:   "inst-1",
			Type:         "serviceTask",
			Status:       store.JobStatusPending,
			ScheduledAt:  time.Now(),
			MaxRetries:   3,
		}
		_ = s.CreateJob(context.Background(), job)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)

	time.Sleep(300 * time.Millisecond)
	cancel()
	wp.Stop()

	mu.Lock()
	p := processed
	mu.Unlock()

	if p < 5 {
		t.Errorf("expected at least 5 jobs processed, got %d", p)
	}
}
