package queue

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/pkg/store"
)

// RetryPolicy defines how failed jobs are retried.
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Jitter     bool
}

// DefaultRetryPolicy returns a sensible default retry policy.
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   5 * time.Minute,
		Jitter:     true,
	}
}

// NextRetryAt calculates when a job should be retried using exponential backoff.
func (rp *RetryPolicy) NextRetryAt(retryCount int) time.Time {
	delay := rp.BaseDelay * time.Duration(math.Pow(2, float64(retryCount)))
	if rp.MaxDelay > 0 && delay > rp.MaxDelay {
		delay = rp.MaxDelay
	}
	if rp.Jitter {
		jitterRange := int64(delay) / 4
		if jitterRange > 0 {
			j, err := rand.Int(rand.Reader, big.NewInt(jitterRange))
			if err == nil {
				delay = delay + time.Duration(j.Int64())
			}
		}
	}
	return time.Now().Add(delay)
}

// ShouldRetry returns true if the job can be retried.
func (rp *RetryPolicy) ShouldRetry(job *store.JobRecord) bool {
	if rp.MaxRetries == 0 {
		return job.RetryCount < job.MaxRetries
	}
	return job.RetryCount < rp.MaxRetries
}

// ApplyRetry updates the job for a retry attempt.
func (rp *RetryPolicy) ApplyRetry(job *store.JobRecord, errMsg string) {
	job.RetryCount++
	job.ErrorMessage = errMsg
	job.Status = store.JobStatusPending
	job.ScheduledAt = rp.NextRetryAt(job.RetryCount)
	job.ExecutedAt = nil
}

// RetryError wraps a retryable error with attempt info.
type RetryError struct {
	Attempt    int
	MaxRetries int
	NextRetry  time.Time
	Err        error
}

func (e *RetryError) Error() string {
	return fmt.Sprintf("job failed (attempt %d/%d), next retry at %s: %v", e.Attempt, e.MaxRetries, e.NextRetry.Format(time.RFC3339), e.Err)
}

func (e *RetryError) Unwrap() error {
	return e.Err
}
