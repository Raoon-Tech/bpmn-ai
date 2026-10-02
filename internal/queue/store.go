package queue

import (
	"context"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// JobStore is the persistence interface for job operations.
type JobStore interface {
	CreateJob(ctx context.Context, job *store.JobRecord) error
	UpdateJob(ctx context.Context, job *store.JobRecord) error
	GetPendingJobs(ctx context.Context, limit int) ([]*store.JobRecord, error)
	GetJob(ctx context.Context, id string) (*store.JobRecord, error)
}

// DeadLetterStore is the persistence interface for dead letter operations.
type DeadLetterStore interface {
	CreateDeadLetter(ctx context.Context, dl *store.DeadLetterRecord) error
	GetDeadLetters(ctx context.Context, instanceID string) ([]*store.DeadLetterRecord, error)
	GetDeadLetter(ctx context.Context, id string) (*store.DeadLetterRecord, error)
	ListDeadLetters(ctx context.Context, limit int) ([]*store.DeadLetterRecord, error)
}
