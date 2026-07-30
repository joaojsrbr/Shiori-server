package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrJobNotFound is returned when a job ID is not found.
	ErrJobNotFound = errors.New("queue: job not found")
	// ErrNoJobs is returned when Dequeue finds no available jobs.
	ErrNoJobs = errors.New("queue: no jobs available")
	// ErrConflict is returned for idempotency conflicts or lease issues.
	ErrConflict = errors.New("queue: conflict or invalid lease")
)

// JobStatus represents the state of a job in the queue.
type JobStatus string

const (
	StatusQueued             JobStatus = "queued"
	StatusRunning            JobStatus = "running"
	StatusRequiresUserAction JobStatus = "requires_user_action"
	StatusRetryScheduled     JobStatus = "retry_scheduled"
	StatusSucceeded          JobStatus = "succeeded"
	StatusSucceededWarnings  JobStatus = "succeeded_with_warnings"
	StatusFailed             JobStatus = "failed"
	StatusCancelled          JobStatus = "cancelled"
)

// Job represents a unit of work.
type Job struct {
	ID             string
	IdempotencyKey string
	Type           string
	Payload        json.RawMessage
	Status         JobStatus
	Priority       int
	Attempts       int
	MaxAttempts    int
	Error          string
	ScheduledAt    time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Provider abstracts the durable queue implementation.
type Provider interface {
	// Enqueue adds a new job to the queue.
	Enqueue(ctx context.Context, job *Job) error

	// Dequeue attempts to acquire a lease on a pending job of the specified types.
	// Returns ErrNoJobs if none are available.
	Dequeue(ctx context.Context, types []string) (*Job, error)

	// Ack marks a job as successfully completed.
	Ack(ctx context.Context, jobID string) error

	// Nack reports a failure. The job may be retried or moved to a dead-letter state.
	Nack(ctx context.Context, jobID string, reason string) error

	// Heartbeat extends the lease for a running job.
	Heartbeat(ctx context.Context, jobID string) error

	// Cancel attempts to cancel a queued or running job.
	Cancel(ctx context.Context, jobID string) error

	// Status retrieves the current status of a job.
	Status(ctx context.Context, jobID string) (*Job, error)
}
