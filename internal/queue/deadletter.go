package queue

import (
	"context"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// DeadLetterQueue stores jobs that exceeded all retry attempts.
type DeadLetterQueue struct {
	store      DeadLetterStore
	jobs       JobStore
	dispatcher *observability.Dispatcher
}

// NewDeadLetterQueue creates a new dead letter queue.
func NewDeadLetterQueue(dl DeadLetterStore, j JobStore) *DeadLetterQueue {
	return &DeadLetterQueue{store: dl, jobs: j}
}

// WithDispatcher sets the event dispatcher for the dead letter queue.
func (dlq *DeadLetterQueue) WithDispatcher(d *observability.Dispatcher) *DeadLetterQueue {
	dlq.dispatcher = d
	return dlq
}

// Add moves a failed job to the dead letter queue.
func (dlq *DeadLetterQueue) Add(ctx context.Context, job *store.JobRecord, errMsg string) error {
	record := &store.DeadLetterRecord{
		JobID:        job.ID,
		InstanceID:   job.InstanceID,
		Type:         job.Type,
		Payload:      job.Payload,
		ErrorMessage: errMsg,
		RetryCount:   job.RetryCount,
	}
	if err := dlq.store.CreateDeadLetter(ctx, record); err != nil {
		return err
	}
	job.Status = store.JobStatusDead
	job.ErrorMessage = errMsg
	if err := dlq.jobs.UpdateJob(ctx, job); err != nil {
		return err
	}

	if dlq.dispatcher != nil {
		dlq.dispatcher.DispatchAsync(observability.Event{
			Type:      observability.EventJobDead,
			Timestamp: time.Now(),
			Payload: map[string]any{
				"instance_id": job.InstanceID,
				"job_id":      job.ID,
				"job_type":    job.Type,
				"error":       errMsg,
			},
		})
	}

	return nil
}

// List returns all dead letters for an instance.
func (dlq *DeadLetterQueue) List(ctx context.Context, instanceID string) ([]*store.DeadLetterRecord, error) {
	return dlq.store.GetDeadLetters(ctx, instanceID)
}

// Get returns a single dead letter by ID.
func (dlq *DeadLetterQueue) Get(ctx context.Context, id string) (*store.DeadLetterRecord, error) {
	return dlq.store.GetDeadLetter(ctx, id)
}

// ListAll returns all dead letters with a limit.
func (dlq *DeadLetterQueue) ListAll(ctx context.Context, limit int) ([]*store.DeadLetterRecord, error) {
	return dlq.store.ListDeadLetters(ctx, limit)
}

// Count returns the total number of dead letters for an instance.
func (dlq *DeadLetterQueue) Count(ctx context.Context, instanceID string) (int, error) {
	dls, err := dlq.store.GetDeadLetters(ctx, instanceID)
	if err != nil {
		return 0, err
	}
	return len(dls), nil
}
