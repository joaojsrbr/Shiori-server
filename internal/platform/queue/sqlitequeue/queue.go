package sqlitequeue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joaojsr/shiori-server/internal/platform/queue"
)

// Provider implements queue.Provider using a SQLite table.
type Provider struct {
	db *sql.DB
}

// New creates a new SQLite-backed queue provider.
func New(db *sql.DB) *Provider {
	return &Provider{db: db}
}

// Enqueue inserts a job. It fails if the IdempotencyKey already exists.
func (p *Provider) Enqueue(ctx context.Context, job *queue.Job) error {
	now := time.Now().UTC()
	if job.ScheduledAt.IsZero() {
		job.ScheduledAt = now
	}
	if job.MaxAttempts == 0 {
		job.MaxAttempts = 3
	}
	job.Status = queue.StatusQueued

	query := `
		INSERT INTO job_queue (
			idempotency_key, job_type, payload, status, priority, 
			max_attempts, scheduled_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`

	var id int64
	err := p.db.QueryRowContext(ctx, query,
		job.IdempotencyKey, job.Type, string(job.Payload), job.Status,
		job.Priority, job.MaxAttempts, job.ScheduledAt.Format(time.RFC3339),
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	).Scan(&id)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return queue.ErrConflict
		}
		return fmt.Errorf("sqlitequeue: enqueue: %w", err)
	}

	job.ID = strconv.FormatInt(id, 10)
	job.CreatedAt = now
	job.UpdatedAt = now
	return nil
}

// Dequeue finds a pending job, generates a lease token, and updates it atomically.
func (p *Provider) Dequeue(ctx context.Context, types []string) (*queue.Job, error) {
	if len(types) == 0 {
		return nil, errors.New("sqlitequeue: no job types specified")
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	leaseToken := uuid.New().String()

	// Determine stale leases (e.g., > 5 minutes without heartbeat)
	staleThreshold := now.Add(-5 * time.Minute).Format(time.RFC3339)

	placeholders := make([]string, len(types))
	args := make([]any, 0, len(types)+3)
	for i, t := range types {
		placeholders[i] = "?"
		args = append(args, t)
	}
	args = append(args, nowStr, staleThreshold)

	// SQLite doesn't have true FOR UPDATE SKIP LOCKED like Postgres, but since this
	// is the portable profile (usually single instance), a simple UPDATE with RETURNING works.
	// We use a subquery to find the best job.
	query := fmt.Sprintf(`
		UPDATE job_queue
		SET 
			status = 'running',
			lease_token = ?,
			leased_at = ?,
			heartbeat_at = ?,
			updated_at = ?,
			attempts = attempts + 1
		WHERE id = (
			SELECT id FROM job_queue
			WHERE job_type IN (%s)
			  AND (
				(status = 'queued' AND scheduled_at <= ?) OR 
				(status = 'running' AND heartbeat_at < ?)
			  )
			ORDER BY priority DESC, scheduled_at ASC
			LIMIT 1
		)
		RETURNING 
			id, idempotency_key, job_type, payload, status, 
			priority, attempts, max_attempts, error, 
			scheduled_at, created_at, updated_at
	`, strings.Join(placeholders, ","))

	fullArgs := append([]any{leaseToken, nowStr, nowStr, nowStr}, args...)

	var j queue.Job
	var payloadStr, schedStr, creStr, updStr string
	var errStr sql.NullString
	var id int64

	err := p.db.QueryRowContext(ctx, query, fullArgs...).Scan(
		&id, &j.IdempotencyKey, &j.Type, &payloadStr, &j.Status,
		&j.Priority, &j.Attempts, &j.MaxAttempts, &errStr,
		&schedStr, &creStr, &updStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, queue.ErrNoJobs
	}
	if err != nil {
		return nil, fmt.Errorf("sqlitequeue: dequeue: %w", err)
	}

	j.ID = strconv.FormatInt(id, 10)
	j.Payload = []byte(payloadStr)
	if errStr.Valid {
		j.Error = errStr.String
	}
	j.ScheduledAt, _ = time.Parse(time.RFC3339, schedStr)
	j.CreatedAt, _ = time.Parse(time.RFC3339, creStr)
	j.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)

	// Since we are not exposing lease token in Job struct (it's internal to the connection or we can just
	// pass it via context if needed, but for MVP we assume ID is enough or we add lease token to Job).
	// To keep it simple, we trust the caller to just use the ID for Ack/Nack in single-node setup.
	return &j, nil
}

// Ack marks the job as succeeded.
func (p *Provider) Ack(ctx context.Context, jobID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := p.db.ExecContext(ctx, `
		UPDATE job_queue 
		SET status = 'succeeded', lease_token = NULL, updated_at = ?
		WHERE id = ? AND status = 'running'
	`, now, jobID)
	if err != nil {
		return fmt.Errorf("sqlitequeue: ack: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return queue.ErrJobNotFound
	}
	return nil
}

// Nack handles failure. If max attempts are reached, it sets status to failed.
func (p *Provider) Nack(ctx context.Context, jobID string, reason string) error {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// Get current attempts and max
	var attempts, max int
	err := p.db.QueryRowContext(ctx, "SELECT attempts, max_attempts FROM job_queue WHERE id = ?", jobID).Scan(&attempts, &max)
	if err != nil {
		return fmt.Errorf("sqlitequeue: nack check: %w", err)
	}

	var newStatus string
	var nextSchedule time.Time
	if attempts >= max {
		newStatus = string(queue.StatusFailed)
		nextSchedule = now
	} else {
		newStatus = string(queue.StatusQueued)
		// Simple exponential backoff (hardcoded 1m, 5m, etc for MVP)
		backoff := time.Duration(attempts*5) * time.Minute
		if backoff == 0 {
			backoff = time.Minute
		}
		nextSchedule = now.Add(backoff)
	}

	_, err = p.db.ExecContext(ctx, `
		UPDATE job_queue 
		SET status = ?, error = ?, lease_token = NULL, scheduled_at = ?, updated_at = ?
		WHERE id = ?
	`, newStatus, reason, nextSchedule.Format(time.RFC3339), nowStr, jobID)

	return err
}

func (p *Provider) Heartbeat(ctx context.Context, jobID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := p.db.ExecContext(ctx, `
		UPDATE job_queue SET heartbeat_at = ?, updated_at = ? WHERE id = ? AND status = 'running'
	`, now, now, jobID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return queue.ErrJobNotFound
	}
	return nil
}

func (p *Provider) Cancel(ctx context.Context, jobID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := p.db.ExecContext(ctx, `
		UPDATE job_queue SET status = 'cancelled', lease_token = NULL, updated_at = ?
		WHERE id = ? AND status IN ('queued','running','requires_user_action','retry_scheduled')
	`, now, jobID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return queue.ErrJobNotFound
	}
	return nil
}

func (p *Provider) Status(ctx context.Context, jobID string) (*queue.Job, error) {
	var j queue.Job
	var payloadStr, schedStr, creStr, updStr string
	var errStr sql.NullString

	err := p.db.QueryRowContext(ctx, `
		SELECT idempotency_key, job_type, payload, status, priority, attempts, max_attempts, error, scheduled_at, created_at, updated_at
		FROM job_queue WHERE id = ?
	`, jobID).Scan(
		&j.IdempotencyKey, &j.Type, &payloadStr, &j.Status, &j.Priority, &j.Attempts, &j.MaxAttempts, &errStr, &schedStr, &creStr, &updStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, queue.ErrJobNotFound
		}
		return nil, err
	}

	j.ID = jobID
	j.Payload = []byte(payloadStr)
	if errStr.Valid {
		j.Error = errStr.String
	}
	j.ScheduledAt, _ = time.Parse(time.RFC3339, schedStr)
	j.CreatedAt, _ = time.Parse(time.RFC3339, creStr)
	j.UpdatedAt, _ = time.Parse(time.RFC3339, updStr)
	return &j, nil
}
